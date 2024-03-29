package appservice

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	appv1 "github.com/lukexwang/luketest-operator/pkg/apis/app/v1"
	"github.com/lukexwang/luketest-operator/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_appservice")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new AppService Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileAppService{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("appservice-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource AppService
	err = c.Watch(&source.Kind{Type: &appv1.AppService{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner AppService
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appv1.AppService{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileAppService implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileAppService{}

// ReconcileAppService reconciles a AppService object
type ReconcileAppService struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a AppService object and makes changes based on the state read
// and what is in the AppService.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileAppService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling AppService")

	// Fetch the AppService instance
	instance := &appv1.AppService{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, err
	}
	deploy := &appsv1.Deployment{}
	if err := r.client.Get(context.TODO(), request.NamespacedName, deploy); err != nil && errors.IsNotFound(err) {
		//创建关联资源
		//1. 创建 Deploy
		deploy := resources.NewDeploy(instance)
		if err := r.client.Create(context.TODO(), deploy); err != nil {
			reqLogger.Error(err, "create deployment fail")
			return reconcile.Result{}, err
		}
		//2.创建service
		service := resources.NewService(instance)
		if err := r.client.Create(context.TODO(), service); err != nil {
			reqLogger.Error(err, "create service fail")
			return reconcile.Result{}, err
		}
		//3. 关联 Annotations
		data, _ := json.Marshal(instance.Spec)
		if instance.Annotations != nil {
			instance.Annotations["spec"] = string(data)
		} else {
			instance.Annotations = map[string]string{"spec": string(data)}
		}

		if err := r.client.Update(context.TODO(), instance); err != nil {
			reqLogger.Error(err, "update instance fail")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	oldSpec := appv1.AppServiceSpec{}
	if err := json.Unmarshal([]byte(instance.Annotations["spec"]), &oldSpec); err != nil {
		reqLogger.Error(err, "json.Unmarshal instance.Annotations[spec] fail")
		return reconcile.Result{}, err
	}
	if !reflect.DeepEqual(instance.Spec, oldSpec) {
		//更新关联资源
		newDeploy := resources.NewDeploy(instance)
		oldDeploy := &appsv1.Deployment{}
		if err := r.client.Get(context.TODO(), request.NamespacedName, oldDeploy); err != nil {
			reqLogger.Error(err, "client.Get old deployment fail")
			return reconcile.Result{}, err
		}
		oldDeploy.Spec = newDeploy.Spec
		if err = r.client.Update(context.TODO(), oldDeploy); err != nil {
			reqLogger.Error(err, "client.Update deployment fail")
			return reconcile.Result{}, err
		}
		reqLogger.Info(fmt.Sprintf("Update deployment success"))

		newService := resources.NewService(instance)
		oldService := &corev1.Service{}
		if err = r.client.Get(context.TODO(), request.NamespacedName, oldService); err != nil {
			reqLogger.Error(err, "client.Get old service fail")
			return reconcile.Result{}, err
		}
		isSvrUpdate := false
		if !reflect.DeepEqual(oldService.Spec.Type, newService.Spec.Type) {
			oldService.Spec.Type = newService.Spec.Type
			isSvrUpdate = true
		}
		if !reflect.DeepEqual(oldService.Spec.Ports, newService.Spec.Ports) {
			oldService.Spec.Ports = newService.Spec.Ports
			isSvrUpdate = true
		}
		if !reflect.DeepEqual(oldService.Spec.Selector, newService.Spec.Selector) {
			oldService.Spec.Selector = newService.Spec.Selector
			isSvrUpdate = true
		}
		if isSvrUpdate == true {
			if err = r.client.Update(context.TODO(), oldService); err != nil {
				reqLogger.Error(err, "client.Update service fail")
				return reconcile.Result{}, err
			}
			reqLogger.Info(fmt.Sprintf("Update service success"))
		}

		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// newPodForCR returns a busybox pod with the same name/namespace as the cr
func newPodForCR(cr *appv1.AppService) *corev1.Pod {
	labels := map[string]string{
		"app": cr.Name,
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-pod",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "busybox",
					Image:   "busybox",
					Command: []string{"sleep", "3600"},
				},
			},
		},
	}
}
