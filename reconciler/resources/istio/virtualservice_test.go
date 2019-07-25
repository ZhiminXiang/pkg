package istio

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis/istio/v1alpha3"
	sharedclientset "knative.dev/pkg/client/clientset/versioned"
	sharedfake "knative.dev/pkg/client/clientset/versioned/fake"
	informers "knative.dev/pkg/client/informers/externalversions"
	fakesharedclient "knative.dev/pkg/client/injection/client/fake"
	"knative.dev/pkg/controller"
	logtesting "knative.dev/pkg/logging/testing"

	. "knative.dev/pkg/reconciler/testing"
)

var ownerObj = &corev1.Service{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "ownerObj",
		Namespace: "default",
		UID:       "abcd",
	},
}
var isControl = true
var ownerRef = metav1.OwnerReference{
	Kind:       ownerObj.Kind,
	Name:       ownerObj.Name,
	UID:        ownerObj.UID,
	Controller: &isControl,
}

var origin = &v1alpha3.VirtualService{
	ObjectMeta: metav1.ObjectMeta{
		Name:            "vs",
		Namespace:       "default",
		OwnerReferences: []metav1.OwnerReference{ownerRef},
	},
	Spec: v1alpha3.VirtualServiceSpec{
		Hosts: []string{"origin.example.com"},
	},
}

var desired = &v1alpha3.VirtualService{
	ObjectMeta: metav1.ObjectMeta{
		Name:            "vs",
		Namespace:       "default",
		OwnerReferences: []metav1.OwnerReference{ownerRef},
	},
	Spec: v1alpha3.VirtualServiceSpec{
		Hosts: []string{"desired.example.com"},
	},
}

func TestReconcileVirtualService_Create(t *testing.T) {
	defer logtesting.ClearAll()
	ctx, _ := SetupFakeContext(t)
	ctx, cancel := context.WithCancel(ctx)
	grp := errgroup.Group{}
	defer func() {
		cancel()
		if err := grp.Wait(); err != nil {
			t.Errorf("Wait() = %v", err)
		}
	}()

	sharedClient := fakesharedclient.Get(ctx)

	h := NewHooks()
	h.OnCreate(&sharedClient.Fake, "virtualservices", func(obj runtime.Object) HookResult {
		got := obj.(*v1alpha3.VirtualService)
		if diff := cmp.Diff(got, desired); diff != "" {
			t.Logf("Unexpected Gateway (-want, +got): %v", diff)
			return HookIncomplete
		}
		return HookComplete
	})

	accessor := setup(ctx, []*v1alpha3.VirtualService{}, sharedClient, t)
	recorder := controller.GetEventRecorder(ctx)
	accessor.ReconcileVirtualService(ctx, ownerObj, desired, recorder)

	if err := h.WaitForHooks(3 * time.Second); err != nil {
		t.Errorf("Failed to Reconcile VirtualService: %v", err)
	}
}

func TestReconcileVirtualService_Update(t *testing.T) {
	defer logtesting.ClearAll()
	ctx, _ := SetupFakeContext(t)
	ctx, cancel := context.WithCancel(ctx)
	grp := errgroup.Group{}
	defer func() {
		cancel()
		if err := grp.Wait(); err != nil {
			t.Errorf("Wait() = %v", err)
		}
	}()

	sharedClient := fakesharedclient.Get(ctx)
	accessor := setup(ctx, []*v1alpha3.VirtualService{origin}, sharedClient, t)

	h := NewHooks()
	h.OnUpdate(&sharedClient.Fake, "virtualservices", func(obj runtime.Object) HookResult {
		got := obj.(*v1alpha3.VirtualService)
		if diff := cmp.Diff(got, desired); diff != "" {
			t.Logf("Unexpected Gateway (-want, +got): %v", diff)
			return HookIncomplete
		}
		return HookComplete
	})

	recorder := controller.GetEventRecorder(ctx)
	accessor.ReconcileVirtualService(ctx, ownerObj, desired, recorder)

	if err := h.WaitForHooks(3 * time.Second); err != nil {
		t.Errorf("Failed to Reconcile VirtualService: %v", err)
	}
}

func setup(ctx context.Context, vses []*v1alpha3.VirtualService,
	sharedClient sharedclientset.Interface, t *testing.T) *VirtualServiceAccessor {

	fake := sharedfake.NewSimpleClientset()
	informer := informers.NewSharedInformerFactory(fake, 0)
	vsInformer := informer.Networking().V1alpha3().VirtualServices()

	for _, vs := range vses {
		fake.NetworkingV1alpha3().VirtualServices(vs.Namespace).Create(vs)
		vsInformer.Informer().GetIndexer().Add(vs)
	}

	if err := controller.StartInformers(ctx.Done(), vsInformer.Informer()); err != nil {
		t.Fatalf("failed to start virtualservice informer: %v", err)
	}

	return &VirtualServiceAccessor{
		SharedClientSet:      sharedClient,
		VirtualServiceLister: vsInformer.Lister(),
	}
}
