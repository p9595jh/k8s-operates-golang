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
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatablev1 "operator/api/v1"
	"operator/queue"
	"operator/resourcer"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var waitDuration = 5 * time.Second

// OperatableReconciler reconciles a Operatable object
type OperatableReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Queue     *queue.Queue[[]byte]
	Resourcer *resourcer.Resourcer
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
	reqLogger := logf.FromContext(ctx).WithValues("req.Namespace", req.Namespace, "req.Name", req.Name)
	reqLogger.Info("Reconciling Operatable")

	// Operatable CR 가져오기
	operatable := &operatablev1.Operatable{}
	err := r.Get(ctx, req.NamespacedName, operatable)
	if err != nil {
		if errors.IsNotFound(err) {
			// 아마도 CR 삭제 케이스
			reqLogger.Info("Operatable resource not found. Ignoring since object must be deleted")
			return ctrl.Result{RequeueAfter: waitDuration}, nil
		}

		reqLogger.Error(err, "Failed to get Operatable CR")
		return ctrl.Result{RequeueAfter: waitDuration}, client.IgnoreNotFound(err)
	}

	// Example logic: Log the desired sizes
	reqLogger.Info("Operatable sizes", "MinSize", operatable.Spec.MinSize, "MaxSize", operatable.Spec.MaxSize, "Replicas", operatable.Status.Replicas)

	// SSA 수행 및 현재 리소스 가져오기
	deployment, _, ssaResult, err := r.ssa(ctx, req, reqLogger, operatable)
	if err != nil {
		return ssaResult, err
	}

	if r.Queue.Len() == 0 {
		reqLogger.Info("No jobs in queue.")

		podList, err := r.getPodList(ctx, &req)
		if err != nil {
			reqLogger.Error(err, "Failed to list pods.", "Operatable.Namespace", operatable.Namespace, "Operatable.Name", operatable.Name)
			return ctrl.Result{RequeueAfter: waitDuration}, err
		}
		for _, pod := range podList.Items {
			reqLogger.Info("Checking the pod working", "pod", pod.Name)
			resp, err := http.Get(fmt.Sprintf("http://%s:8070/api/jobs/v1", pod.Status.PodIP))
			if err != nil {
				reqLogger.Error(err, "Failed to get jobs", "pod", pod.Name)
				return ctrl.Result{RequeueAfter: waitDuration}, err
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				reqLogger.Error(err, "Failed to read body", "pod", pod.Name)
				return ctrl.Result{RequeueAfter: waitDuration}, err
			}
			defer resp.Body.Close()

			reqLogger.Info("Got the pod status", "pod", pod.Name, "status", resp.StatusCode, "body", string(body))
			if resp.StatusCode != http.StatusNotFound {
				// 아직 처리중인 job이 있음
				return ctrl.Result{RequeueAfter: waitDuration}, nil
			}
		}

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.Get(ctx, req.NamespacedName, deployment); err != nil {
				return err
			}
			deployment.Spec.Replicas = &operatable.Spec.MinSize
			err = r.Client.Update(ctx, deployment)
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			reqLogger.Error(err, "Failed to scale down Deployment.", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
			return ctrl.Result{RequeueAfter: waitDuration}, err
		}
		return ctrl.Result{RequeueAfter: waitDuration}, nil
	}

	reqLogger.Info("Jobs in queue.", "Queue.Len", r.Queue.Len())

	podList, err := r.getPodList(ctx, &req)
	if err != nil {
		reqLogger.Error(err, "Failed to list pods.", "Operatable.Namespace", operatable.Namespace, "Operatable.Name", operatable.Name)
		return ctrl.Result{RequeueAfter: waitDuration}, err
	}

	for _, pod := range podList.Items {

	POD_CHECKING:
		for {
			var conditions []string
			if pod.Status.Phase == corev1.PodRunning && pod.Status.PodIP != "" {
				conditions = make([]string, 0, len(pod.Status.Conditions))
				for _, cond := range pod.Status.Conditions {
					if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
						break POD_CHECKING
					}
					if cond.Type != "" && cond.Status != "" {
						conditions = append(conditions, fmt.Sprintf("%s=%s", cond.Type, cond.Status))
					}
				}
			}

			reqLogger.Info(
				"Get pod property",
				"pod", pod.Name,
				"phase", pod.Status.Phase,
				"podIP", pod.Status.PodIP,
				"conditions", conditions,
			)
			time.Sleep(500 * time.Millisecond)
			if err := r.reloadPod(ctx, &pod); err != nil {
				reqLogger.Error(err, "Failed to reload pod", "pod", pod.Name)
			}
		}

		// 작업 처리 중인지 확인
		getResp, err := http.Get(fmt.Sprintf("http://%s:8070/api/jobs/v1", pod.Status.PodIP))
		if err != nil {
			reqLogger.Error(err, "Failed to get jobs", "pod", pod.Name)
			return ctrl.Result{RequeueAfter: waitDuration}, err
		}

		getBody, err := io.ReadAll(getResp.Body)
		if err != nil {
			return ctrl.Result{RequeueAfter: waitDuration}, err
		}
		defer getResp.Body.Close()

		reqLogger.Info("Got pod status", "pod", pod.Name, "status", getResp.StatusCode, "body", string(getBody))
		if getResp.StatusCode != http.StatusNotFound {
			continue
		}

		// 작업 할당
		job, _ := r.Queue.Pop()
		postResp, err := http.Post(fmt.Sprintf("http://%s:8070/api/jobs/v1", pod.Status.PodIP), "application/json", bytes.NewBuffer(job))
		if err != nil {
			reqLogger.Error(err, "Failed to post body", "pod", pod.Name)
			continue
		}
		if postResp.StatusCode != http.StatusCreated {
			reqLogger.Error(err, "Failed to create job", "pod", pod.Name, "status", postResp.StatusCode)
			continue
		}
		defer postResp.Body.Close()

		body, err := io.ReadAll(postResp.Body)
		if err != nil {
			reqLogger.Error(err, "Failed to parse body", "pod", pod.Name)
			continue
		}

		reqLogger.Info("Job assigned", "pod", pod.Name, "job", string(job), "body", string(body))
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.Get(ctx, req.NamespacedName, deployment); err != nil {
			return err
		}

		if *deployment.Spec.Replicas == operatable.Spec.MaxSize {
			return nil
		}

		*deployment.Spec.Replicas = min(*deployment.Spec.Replicas+int32(r.Queue.Len()), operatable.Spec.MaxSize)
		err = r.Client.Update(ctx, deployment)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		reqLogger.Error(err, "Failed to scale up Deployment.", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		return ctrl.Result{RequeueAfter: waitDuration}, err
	}

	return ctrl.Result{RequeueAfter: waitDuration}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OperatableReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatablev1.Operatable{}).
		Named("operatable").
		Complete(r)
}

func (r *OperatableReconciler) ssa(
	ctx context.Context,
	req ctrl.Request,
	reqLogger logr.Logger,
	operatable *operatablev1.Operatable,
) (*appsv1.Deployment, *corev1.Service, ctrl.Result, error) {

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
		reqLogger.Error(err, "Failed to set controller reference on Deployment")
		return nil, nil, ctrl.Result{RequeueAfter: waitDuration}, err
	}

	// Apply 실행 (없으면 생성, 있으면 수정)
	// PatchOption 순서 중요: Patch 메서드의 인자로 전달
	if err := r.Patch(ctx, dep, client.Apply, client.FieldOwner("operatable-controller"), client.ForceOwnership); err != nil {
		reqLogger.Error(err, "Failed to apply Deployment")
		return nil, nil, ctrl.Result{RequeueAfter: waitDuration}, err
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
		reqLogger.Error(err, "Failed to set controller reference on Service")
		return nil, nil, ctrl.Result{RequeueAfter: waitDuration}, err
	}

	if err := r.Patch(ctx, svc, client.Apply, client.FieldOwner("operatable-controller"), client.ForceOwnership); err != nil {
		reqLogger.Error(err, "Failed to apply Service")
		return nil, nil, ctrl.Result{RequeueAfter: waitDuration}, err
	}

	// ====================================================================
	// Get Currents
	// ====================================================================

	currentDep := &appsv1.Deployment{}
	if err := r.Get(ctx, req.NamespacedName, currentDep); err != nil {
		reqLogger.Error(err, "Failed to get current Deployment")
		return nil, nil, ctrl.Result{RequeueAfter: waitDuration}, err
	}

	currentService := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, currentService); err != nil {
		reqLogger.Error(err, "Failed to get current Service")
		return nil, nil, ctrl.Result{RequeueAfter: waitDuration}, err
	}

	return currentDep, currentService, ctrl.Result{}, nil
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

func (r *OperatableReconciler) reloadPod(ctx context.Context, pod *corev1.Pod) error {
	return r.Client.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, pod)
}
