package status

import (
	"context"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	addonv1alpha1 "sigs.k8s.io/controller-runtime/alpha/patterns/addon/pkg/apis/v1alpha1"
	"sigs.k8s.io/controller-runtime/alpha/patterns/declarative"
	"sigs.k8s.io/controller-runtime/alpha/patterns/declarative/pkg/manifest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const successfulDeployment = appsv1.DeploymentAvailable

// NewAggregator provides an implementation of declarative.Reconciled that
// aggregates the status of deployed objects to configure the 'Healthy'
// field on an addon that derives from CommonStatus
func NewAggregator(client client.Client) *aggregator {
	return &aggregator{client}
}

type aggregator struct {
	client client.Client
}

func (a *aggregator) Reconciled(ctx context.Context, src declarative.DeclarativeObject, objs *manifest.Objects) error {
	log := log.Log

	instance, ok := src.(addonv1alpha1.CommonObject)
	if !ok {
		return fmt.Errorf("object %T was not an addonv1alpha1.CommonObject", src)
	}

	status := addonv1alpha1.CommonStatus{Healthy: true}

	for _, o := range objs.Items {
		gk := o.Group + "/" + o.Kind
		healthy := true
		var err error
		switch gk {
		case "/Service":
			healthy, err = a.service(ctx, instance, o.Name)
		case "extensions/Deployment", "apps/Deployment":
			healthy, err = a.deployment(ctx, instance, o.Name)
		default:
			log.WithValues("type", gk).Info("type not implemented for status aggregation, skipping")
		}

		status.Healthy = status.Healthy && healthy
		if err != nil {
			status.Errors = append(status.Errors, fmt.Sprintf("%v", err))
		}
	}

	log.WithValues("object", src).WithValues("status", status).V(2).Info("built status")

	if !reflect.DeepEqual(status, instance.GetCommonStatus()) {
		instance.SetCommonStatus(status)

		log.WithValues("name", instance.GetName()).WithValues("status", status).Info("updating status")

		err := a.client.Update(ctx, instance)
		if err != nil {
			log.Error(err, "updating status")
			return err
		}
	}

	return nil

	/*
		var phase applicationv1beta1.ApplicationAssemblyPhase
		if status.Healthy {
			phase = applicationv1beta1.Succeeded
		} else {
			phase = applicationv1beta1.Pending
		}

		if app.Spec.AssemblyPhase != phase {
			app.Spec.AssemblyPhase = phase
			if _, err := r.applicationClient.AppV1beta1().Applications(name.Namespace).Update(app); err != nil {
				log.Error(err, "updating assembly phase on application")
			}
		}
	*/
}

func (a *aggregator) deployment(ctx context.Context, src addonv1alpha1.CommonObject, name string) (bool, error) {
	key := client.ObjectKey{src.GetNamespace(), name}
	dep := &appsv1.Deployment{}

	if err := a.client.Get(ctx, key, dep); err != nil {
		return false, fmt.Errorf("error reading deployment (%s): %v", key, err)
	}

	for _, cond := range dep.Status.Conditions {
		if cond.Type == successfulDeployment && cond.Status == corev1.ConditionTrue {
			return true, nil
		}
	}

	return false, fmt.Errorf("deployment (%s) does not meet condition: %s", key, successfulDeployment)
}

func (a *aggregator) service(ctx context.Context, src addonv1alpha1.CommonObject, name string) (bool, error) {
	key := client.ObjectKey{src.GetNamespace(), name}
	svc := &corev1.Service{}
	err := a.client.Get(ctx, key, svc)
	if err != nil {
		return false, fmt.Errorf("error reading service (%s): %v", key, err)
	}

	return true, nil
}
