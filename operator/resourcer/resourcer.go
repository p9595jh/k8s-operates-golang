package resourcer

import (
	"bytes"
	"fmt"
	"html/template"
	"os"

	"go.yaml.in/yaml/v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

const (
	chartFolder    = "operatable-chart"
	templateFolder = "templates"
)

type Resourcer struct {
	Deployment *appsv1.Deployment
	Service    *corev1.Service
	Ingress    *networkingv1.Ingress
	ConfigMap  *corev1.ConfigMap
	PVC        *corev1.PersistentVolumeClaim
}

func New() (*Resourcer, error) {
	valuesBytes, err := os.ReadFile(fmt.Sprintf("./%s/values.yaml", chartFolder))
	if err != nil {
		return nil, err
	}

	var values map[string]any
	err = yaml.Unmarshal(valuesBytes, &values)
	if err != nil {
		return nil, err
	}
	values = map[string]any{"Values": values}

	var (
		deployment appsv1.Deployment
		service    corev1.Service
		ingress    networkingv1.Ingress
		configMap  corev1.ConfigMap
		pvc        corev1.PersistentVolumeClaim
	)
	if err := tmplExec("deployment", values, &deployment); err != nil {
		return nil, err
	}
	if err := tmplExec("service", values, &service); err != nil {
		return nil, err
	}
	// if err := tmplExec("ingress", valuesData, &ingress); err != nil {
	// 	return nil, err
	// }
	// if err := tmplExec("configmap", valuesData, &configMap); err != nil {
	// 	return nil, err
	// }
	// if err := tmplExec("pvc", valuesData, &pvc); err != nil {
	// 	return nil, err
	// }

	return &Resourcer{
		Deployment: &deployment,
		Service:    &service,
		Ingress:    &ingress,
		ConfigMap:  &configMap,
		PVC:        &pvc,
	}, nil
}

func tmplExec(name string, values map[string]any, target any) error {
	tmpl, err := template.ParseFiles(fmt.Sprintf("./%s/%s/%s.yaml", chartFolder, templateFolder, name))
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", name, err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, values)
	if err != nil {
		return fmt.Errorf("failed to execute %s: %w", name, err)
	}

	err = k8syaml.Unmarshal(buf.Bytes(), target)
	if err != nil {
		return fmt.Errorf("failed to unmarshal %s: %w", name, err)
	}

	return nil
}
