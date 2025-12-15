/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"syscall"

	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog/log"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	operatablev1 "operator/api/v1"
	"operator/model"
	"operator/queue"
	"operator/resourcer"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OperatableReconciler reconciles a Operatable object
type OperatableReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Queue        *queue.Queue[*model.JobData]
	Reservations map[string]*model.JobData // map[PodName]JobID
	Resourcer    *resourcer.Resourcer
	EventChannel chan event.GenericEvent
}

// +kubebuilder:rbac:groups=app.p9595jh.com,resources=operatables,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=app.p9595jh.com,resources=operatables/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=app.p9595jh.com,resources=operatables/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps;secrets;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Operatable object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *OperatableReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.With().Str("rid", ulid.Make().String()).Logger()
	log.Info().Msg("Reconciling Operatable")

	// Operatable CR 가져오기
	operatable := &operatablev1.Operatable{}
	err := r.Get(ctx, req.NamespacedName, operatable)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info().Msg("Operatable resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error().Err(err).Msg("Failed to get Operatable CR")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// SSA 수행 및 현재 리소스 가져오기
	deployment, _, err := r.ssa(ctx, req, operatable)
	if err != nil {
		log.Error().Err(err).Msg("Failed to apply resources via SSA")
		return ctrl.Result{}, err
	}

	log.Info().
		Int32("MinSize", operatable.Spec.MinSize).
		Int32("MaxSize", operatable.Spec.MaxSize).
		Int32("Status.Replicas", deployment.Status.Replicas).
		Int32("Spec.Replicas", *deployment.Spec.Replicas).
		Msg("Operatable sizes")

	log.Info().Int("Queue.Len", r.Queue.Len()).Int("Reservations", len(r.Reservations)).Msg("Jobs in queue.")

	podList, err := r.getPodList(ctx, &req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list pods.")
		return ctrl.Result{}, err
	}

	// 🔥 [수정] 현재 작업 중인 파드 개수를 세기 위한 변수 추가
	busyWorkers := 0

	for _, pod := range podList.Items {
		log.Info().Str("pod", pod.Name).Msg("Checking the pod")

		// 1. Starting 상태인 파드 처리 (예약 로직)
		if isPodStarting(&pod) {
			if _, reserved := r.Reservations[pod.Name]; !reserved {
				// 큐에 작업이 없으면 예약 불필요
				if r.Queue.Len() == 0 {
					// 예약은 안 했지만, Starting 중이므로 이 루프는 종료
					// (단, busyWorkers 카운트는 하지 않음 - Reservations로 계산할 것이므로)
					continue
				}

				job, ok := r.Queue.Pop()
				if !ok || job == nil {
					continue
				}

				r.Reservations[pod.Name] = job
				log.Info().Msgf("Job %s is reserved for pod %s during startup.", job.ID, pod.Name)
			}
			// 이미 예약되었거나 방금 예약한 경우
			// 나중에 len(r.Reservations)로 계산할 것이므로 여기선 카운트 X (또는 로직에 따라 포함 가능)
			continue
		}

		if !isPodReady(&pod) {
			log.Info().Str("pod", pod.Name).Msg("Pod is not ready, skipping.")
			continue
		}

		// 2. Running 상태인 파드 작업 확인
		// 작업 처리 중인지 확인
		getResp, err := http.Get(fmt.Sprintf("http://%s:8070/api/jobs/v1", pod.Status.PodIP))
		if err != nil {
			if errors.Is(err, syscall.ECONNREFUSED) {
				continue
			}
			log.Error().Err(err).Str("pod", pod.Name).Msg("Failed to get jobs")
			// 통신 에러가 나면 일단 busy로 간주하지 않음 (안전하게)
			continue
		}
		defer getResp.Body.Close()

		getBody, err := io.ReadAll(getResp.Body)
		if err != nil {
			log.Error().Err(err).Str("pod", pod.Name).Msg("Failed to read job response body")
			return ctrl.Result{}, err
		}

		log.Info().Str("pod", pod.Name).Int("status", getResp.StatusCode).RawJSON("body", getBody).Msg("Got pod status")

		// 🔥 [수정] 작업 중이라면 카운트 증가
		if getResp.StatusCode != http.StatusNotFound {
			busyWorkers++ // 이 파드는 바쁩니다.
			continue
		}

		// 3. 노는(Idle) 파드에게 작업 할당
		var job *model.JobData
		if reservedJob, reserved := r.Reservations[pod.Name]; reserved {
			// 예약된 작업이 있으면 사용
			job = reservedJob
			delete(r.Reservations, pod.Name)
			log.Info().Msgf("Using reserved job %s for pod %s.", job.ID, pod.Name)
		} else {
			if r.Queue.Len() == 0 {
				continue // 더 줄 작업이 없음
			}
			job, _ = r.Queue.Pop()
		}

		// 작업 전송
		postResp, err := http.Post(fmt.Sprintf("http://%s:8070/api/jobs/v1", pod.Status.PodIP), "application/json", job.Data.ToBuffer())
		if err != nil {
			log.Error().Err(err).Str("pod", pod.Name).Msg("Failed to post job")
			// 실패 시 큐에 다시 넣는 로직 고려 필요 (여기선 생략)
			continue
		}
		defer postResp.Body.Close()

		if postResp.StatusCode == http.StatusCreated {
			// 작업을 할당했으니, 이 파드도 이제 "Busy" 입니다.
			busyWorkers++
			body, _ := io.ReadAll(postResp.Body)
			log.Info().Str("pod", pod.Name).Any("job", job).RawJSON("body", body).Msg("Job assigned")
		} else {
			log.Error().Str("pod", pod.Name).Int("status", postResp.StatusCode).Msg("Failed to create job")
		}
	}

	// ====================================================================
	// Scaling Logic (Differential Scale)
	// ====================================================================

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.Get(ctx, req.NamespacedName, deployment); err != nil {
			return err
		}

		// 🔥 [핵심 수정] 목표 레플리카 수 계산 공식 변경
		// Target = (현재 작업 중인 파드) + (준비 중인 파드) + (대기 중인 작업)
		// 이렇게 하면 Reconcile이 100번 돌아도 중복으로 더해지지 않습니다.
		totalNeeded := busyWorkers + len(r.Reservations) + r.Queue.Len()

		// Min/Max 적용
		desiredReplicas := min(max(int32(totalNeeded), operatable.Spec.MinSize), operatable.Spec.MaxSize)

		// 변경 사항이 없으면 리턴
		if *deployment.Spec.Replicas == desiredReplicas {
			return nil
		}

		log.Info().
			Int("Busy", busyWorkers).
			Int("Reserved", len(r.Reservations)).
			Int("Queue", r.Queue.Len()).
			Int32("Current", *deployment.Spec.Replicas).
			Int32("Target", desiredReplicas).
			Msg("Scaling Deployment")

		deployment.Spec.Replicas = &desiredReplicas
		return r.Client.Update(ctx, deployment)
	})

	if err != nil {
		log.Error().Err(err).Msg("Failed to update Deployment.")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OperatableReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.EventChannel = make(chan event.GenericEvent, 128)

	// 디버깅용 Predicate 정의
	logEvents := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			log.Info().Str("kind", e.Object.GetObjectKind().GroupVersionKind().Kind).Msg("🟢 Create Event detected")
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			log.Info().Msg("🔴 Delete Event detected")
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Spec이 바뀌었는지, Status가 바뀌었는지 확인 가능
			oldGen := e.ObjectOld.GetGeneration()
			newGen := e.ObjectNew.GetGeneration()
			if oldGen != newGen {
				log.Info().Int64("old", oldGen).Int64("new", newGen).Msg("🟡 Spec Update detected")
			} else {
				log.Info().Msg("🟡 Status/Meta Update detected")
			}
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			// 🔥 채널(EventChannel)을 통해 들어온 건 여기서 잡힘!
			log.Info().Msg("🟣 Generic/Channel Event detected")
			return true
		},
	}
	_ = logEvents

	return ctrl.NewControllerManagedBy(mgr).
		For(&operatablev1.Operatable{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Pod{}, builder.WithPredicates(podReadyPredicate())).
		WatchesRawSource(
			source.Channel(r.EventChannel, &handler.EnqueueRequestForObject{}),
		).
		Named("operatable").
		Complete(r)
}

func (r *OperatableReconciler) ssa(
	ctx context.Context,
	req ctrl.Request,
	operatable *operatablev1.Operatable,
) (*appsv1.Deployment, *corev1.Service, error) {

	// ====================================================================
	// Deployment Apply (Server-Side Apply)
	// ====================================================================

	// 템플릿 원본 보호를 위해 DeepCopy 사용
	dep := r.Resourcer.Deployment.DeepCopy()

	// SSA 필수 설정: TypeMeta, Name, Namespace
	dep.APIVersion = "apps/v1"
	dep.Kind = "Deployment"
	dep.Name = req.Name
	dep.Namespace = req.Namespace

	// [중요] Replicas 관리 권한 포기
	// SSA가 실행될 때마다 replicas를 템플릿 값(예: 1)으로 덮어쓰지 않도록 nil로 설정합니다.
	// 이렇게 하면 아래쪽의 오토스케일링 로직이 설정한 값을 유지할 수 있습니다.
	dep.Spec.Replicas = nil

	// OwnerReference 설정
	if err := ctrl.SetControllerReference(operatable, dep, r.Scheme); err != nil {
		return nil, nil, fmt.Errorf("failed to set controller reference on Deployment: %w", err)
	}

	// Apply 실행 (없으면 생성, 있으면 수정)
	// PatchOption 순서 중요: Patch 메서드의 인자로 전달
	if err := r.Patch(ctx, dep, client.Apply, client.FieldOwner("operatable-controller"), client.ForceOwnership); err != nil {
		return nil, nil, fmt.Errorf("failed to apply Deployment: %w", err)
	}

	// ====================================================================
	// Service Apply (Server-Side Apply)
	// ====================================================================

	svc := r.Resourcer.Service.DeepCopy()

	svc.APIVersion = "v1"
	svc.Kind = "Service"
	svc.Name = req.Name
	svc.Namespace = req.Namespace

	if err := ctrl.SetControllerReference(operatable, svc, r.Scheme); err != nil {
		return nil, nil, fmt.Errorf("failed to set controller reference on Service: %w", err)
	}

	if err := r.Patch(ctx, svc, client.Apply, client.FieldOwner("operatable-controller"), client.ForceOwnership); err != nil {
		return nil, nil, fmt.Errorf("failed to apply Service: %w", err)
	}

	// ====================================================================
	// Get Currents
	// ====================================================================

	currentDep := &appsv1.Deployment{}
	if err := r.Get(ctx, req.NamespacedName, currentDep); err != nil {
		return nil, nil, fmt.Errorf("failed to get current Deployment: %w", err)
	}

	currentService := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, currentService); err != nil {
		return nil, nil, fmt.Errorf("failed to get current Service: %w", err)
	}

	return currentDep, currentService, nil
}

func (r *OperatableReconciler) getPodList(ctx context.Context, req *ctrl.Request) (*corev1.PodList, error) {
	podList := &corev1.PodList{}
	err := r.Client.List(
		ctx,
		podList,
		client.InNamespace(req.NamespacedName.Namespace),
		client.MatchingLabels(map[string]string{"app": "operatable"}),
	)
	if err != nil {
		return nil, err
	}
	return podList, nil
}

func (r *OperatableReconciler) NotifyEvent(namespace, name string) {
	log.Info().Str("cr", name).Msg("🟣 Triggering Reconcile via Channel")

	select {
	case r.EventChannel <- event.GenericEvent{
		Object: &operatablev1.Operatable{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}:
	default:
	}
}
