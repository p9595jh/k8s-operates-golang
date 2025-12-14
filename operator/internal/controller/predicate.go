package controller

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func isPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodRunning && pod.Status.PodIP != "" {
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

func isPodStarting(pod *corev1.Pod) bool {
	// 1. 삭제 중인 경우 (Terminating) -> 뜨는 중 아님
	if pod.DeletionTimestamp != nil {
		return false
	}

	// 2. 이미 작업이 완료되었거나(Succeeded) 실패한(Failed) 경우 -> 뜨는 중 아님
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return false
	}

	// 3. 이미 준비 완료된(Ready) 경우 -> 뜨는 중 아님 (이미 다 떴음)
	if isPodReady(pod) {
		return false
	}

	// 4. 남은 경우: Pending 상태이거나, Running이지만 아직 Ready가 아닌 상태
	// (이미지 풀링 중, 컨테이너 생성 중, 앱 초기화 중 등)
	return true
}

// 파드가 "준비 완료(Ready)" 상태로 변할 때만 이벤트를 발생시키는 필터
func podReadyPredicate() predicate.Predicate {
	return predicate.Funcs{
		// 1. 생성 이벤트: 이미 Ready 상태로 생성된 경우(드물지만) 처리
		CreateFunc: func(e event.CreateEvent) bool {
			pod, ok := e.Object.(*corev1.Pod)
			if !ok {
				return false
			}
			return isPodReady(pod)
		},
		// 2. 삭제 이벤트
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		// 3. 수정 이벤트: [핵심] NotReady -> Ready로 바뀔 때만 true
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPod, ok1 := e.ObjectOld.(*corev1.Pod)
			newPod, ok2 := e.ObjectNew.(*corev1.Pod)
			if !ok1 || !ok2 {
				return false
			}

			// 이전에는 Not Ready 였는데, 지금 Ready가 되었다면 트리거!
			if !isPodReady(oldPod) && isPodReady(newPod) {
				return true
			}

			// // (선택) 반대로 Ready였다가 죽은 경우도 감지하고 싶다면 추가
			// if isPodReady(oldPod) && !isPodReady(newPod) {
			// 	return true
			// }

			// 그 외(그냥 라벨 변경, 잡다한 status 변경 등)는 무시
			return false
		},
		// 4. 일반적인 Generic 이벤트
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}
