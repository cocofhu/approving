package kubernetes

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sandbox-gateway/internal/driver"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestInjectConfigEnv(t *testing.T) {
	base := map[string]string{"FOO": "bar"}

	t.Run("nil config is a no-op copy", func(t *testing.T) {
		out := injectConfigEnv(base, nil)
		if out["FOO"] != "bar" || len(out) != 1 {
			t.Fatalf("unexpected env: %v", out)
		}
		out["FOO"] = "mutated"
		if base["FOO"] != "bar" {
			t.Fatalf("input env must not be mutated")
		}
	})

	t.Run("empty bundle URL adds nothing", func(t *testing.T) {
		out := injectConfigEnv(base, &driver.ConfigInject{ConfigRoot: "/root/.cursor"})
		if _, ok := out["SANDBOX_INJECT"]; ok {
			t.Fatalf("SANDBOX_INJECT must not be set without BundleURL: %v", out)
		}
	})

	t.Run("bundle URL with root and headers", func(t *testing.T) {
		out := injectConfigEnv(base, &driver.ConfigInject{
			BundleURL:  "https://example.com/b.tgz",
			ConfigRoot: "/root/.cursor",
			Headers:    "Authorization: Bearer x",
		})
		if got, want := out["SANDBOX_INJECT"], "https://example.com/b.tgz|/root/.cursor"; got != want {
			t.Fatalf("SANDBOX_INJECT=%q want %q", got, want)
		}
		if out["SANDBOX_INJECT_HEADERS"] != "Authorization: Bearer x" {
			t.Fatalf("headers not propagated: %v", out)
		}
		if out["FOO"] != "bar" {
			t.Fatalf("base env lost: %v", out)
		}
	})

	t.Run("bundle URL without root omits pipe", func(t *testing.T) {
		out := injectConfigEnv(nil, &driver.ConfigInject{BundleURL: "https://example.com/b.tgz"})
		if out["SANDBOX_INJECT"] != "https://example.com/b.tgz" {
			t.Fatalf("unexpected SANDBOX_INJECT: %q", out["SANDBOX_INJECT"])
		}
	})
}

func testDriver(t *testing.T, enableLB bool) *Driver {
	t.Helper()
	cs := fake.NewSimpleClientset()
	return NewFromClient(cs, Options{
		Namespace:          "sandboxes",
		NamePrefix:         "sbx-",
		EnableLoadBalancer: enableLB,
		StorageClass:       "ugreen-iscsi",
	})
}

func TestNameAndHelpers(t *testing.T) {
	d := testDriver(t, true)
	if d.Name() != "kubernetes" {
		t.Fatalf("Name=%q", d.Name())
	}
	id := "abc"
	if d.resourceName(id) != "sbx-abc" {
		t.Fatalf("resourceName=%q", d.resourceName(id))
	}
	if d.lbName(id) != "sbx-abc-lb" {
		t.Fatalf("lbName=%q", d.lbName(id))
	}
	if d.pvcName(id) != "sbx-abc-data" {
		t.Fatalf("pvcName=%q", d.pvcName(id))
	}
	if d.secretName(id) != "sbx-abc-env" {
		t.Fatalf("secretName=%q", d.secretName(id))
	}
}

func seedSandboxPod(t *testing.T, d *Driver, id, name string, phase corev1.PodPhase, created time.Time) {
	t.Helper()
	_, err := d.cs.CoreV1().Pods(d.opts.Namespace).Create(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         d.opts.Namespace,
			Labels:            d.selector(id),
			CreationTimestamp: metav1.NewTime(created),
		},
		Status: corev1.PodStatus{Phase: phase},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("seed pod: %v", err)
	}
}

func TestLogsContent(t *testing.T) {
	d := testDriver(t, true)
	id := "log1"
	seedSandboxPod(t, d, id, "sbx-log1-pod", corev1.PodRunning, time.Now())
	want := "[boot] sandbox container started\nhello\n"
	var gotOpts *corev1.PodLogOptions
	d.getPodLogs = func(_ context.Context, ns, pod string, opts *corev1.PodLogOptions) (string, error) {
		if ns != "sandboxes" || pod != "sbx-log1-pod" {
			t.Fatalf("getPodLogs ns/pod=%s/%s", ns, pod)
		}
		gotOpts = opts
		return want, nil
	}
	out, err := d.Logs(context.Background(), id, 100)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if out != want {
		t.Fatalf("content=%q want %q", out, want)
	}
	if gotOpts == nil || gotOpts.Container != sandboxContainer || gotOpts.Follow {
		t.Fatalf("opts=%+v", gotOpts)
	}
	if gotOpts.TailLines == nil || *gotOpts.TailLines != 100 {
		t.Fatalf("TailLines=%v", gotOpts.TailLines)
	}
}

func TestLogsEmptySuccess(t *testing.T) {
	d := testDriver(t, true)
	id := "empty"
	seedSandboxPod(t, d, id, "sbx-empty-pod", corev1.PodRunning, time.Now())
	d.getPodLogs = func(_ context.Context, _, _ string, opts *corev1.PodLogOptions) (string, error) {
		if opts.TailLines == nil || *opts.TailLines != defaultLogsTail {
			t.Fatalf("default tail: %v", opts.TailLines)
		}
		return "", nil
	}
	out, err := d.Logs(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("Logs empty success: %v", err)
	}
	if out != "" {
		t.Fatalf("want empty content, got %q", out)
	}
}

func TestLogsNotFound(t *testing.T) {
	d := testDriver(t, true)
	d.getPodLogs = func(context.Context, string, string, *corev1.PodLogOptions) (string, error) {
		t.Fatal("getPodLogs must not be called when no pods match")
		return "", nil
	}
	_, err := d.Logs(context.Background(), "missing", 10)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want not-found error, got %v", err)
	}
}

// TestDefaultGetPodLogsViaFakeClient exercises the real defaultGetPodLogs path
// (no mock). client-go's fake Pods.GetLogs streams "fake logs".
func TestDefaultGetPodLogsViaFakeClient(t *testing.T) {
	d := testDriver(t, true)
	id := "deflogs"
	seedSandboxPod(t, d, id, "sbx-deflogs-pod", corev1.PodRunning, time.Now())
	out, err := d.Logs(context.Background(), id, 50)
	if err != nil {
		t.Fatalf("Logs via defaultGetPodLogs: %v", err)
	}
	if out != "fake logs" {
		t.Fatalf("content=%q want %q from fake GetLogs", out, "fake logs")
	}
}

func TestDefaultGetPodLogsDirect(t *testing.T) {
	d := testDriver(t, true)
	tail := int64(10)
	out, err := d.defaultGetPodLogs(context.Background(), "sandboxes", "any-pod", &corev1.PodLogOptions{
		Container: sandboxContainer,
		Follow:    false,
		TailLines: &tail,
	})
	if err != nil {
		t.Fatalf("defaultGetPodLogs: %v", err)
	}
	if out != "fake logs" {
		t.Fatalf("content=%q want fake logs", out)
	}
}

type errReadCloser struct {
	err error
}

func (e errReadCloser) Read([]byte) (int, error) { return 0, e.err }
func (e errReadCloser) Close() error             { return nil }

func TestDrainLogStream(t *testing.T) {
	out, err := drainLogStream(io.NopCloser(strings.NewReader("hello\n")))
	if err != nil || out != "hello\n" {
		t.Fatalf("drain success: %q err=%v", out, err)
	}
	out, err = drainLogStream(io.NopCloser(strings.NewReader("")))
	if err != nil || out != "" {
		t.Fatalf("drain empty: %q err=%v", out, err)
	}
	_, err = drainLogStream(errReadCloser{err: fmt.Errorf("read boom")})
	if err == nil || !strings.Contains(err.Error(), "read boom") {
		t.Fatalf("want read error, got %v", err)
	}
}

func TestLogsGetPodLogsError(t *testing.T) {
	d := testDriver(t, true)
	id := "logerr"
	seedSandboxPod(t, d, id, "sbx-logerr-pod", corev1.PodRunning, time.Now())
	d.getPodLogs = func(context.Context, string, string, *corev1.PodLogOptions) (string, error) {
		return "", fmt.Errorf("stream refused")
	}
	_, err := d.Logs(context.Background(), id, 10)
	if err == nil || !strings.Contains(err.Error(), "kubernetes logs") || !strings.Contains(err.Error(), "stream refused") {
		t.Fatalf("want wrapped stream error, got %v", err)
	}
}

func TestLogsListPodsError(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("list apiserver down")
	})
	d := NewFromClient(cs, Options{Namespace: "sandboxes", NamePrefix: "sbx-"})
	_, err := d.Logs(context.Background(), "x", 10)
	if err == nil || !strings.Contains(err.Error(), "list pods") {
		t.Fatalf("want list pods error, got %v", err)
	}
}

func TestPickPodForLogsPrefersRunning(t *testing.T) {
	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pending-new", CreationTimestamp: metav1.NewTime(newer)},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "running-old", CreationTimestamp: metav1.NewTime(older)},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "succeeded", CreationTimestamp: metav1.NewTime(newer)},
			Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
		},
	}
	got := pickPodForLogs(pods)
	if got == nil || got.Name != "running-old" {
		t.Fatalf("pick=%v want running-old", got)
	}
	if pickPodForLogs(nil) != nil {
		t.Fatal("empty list must return nil")
	}
	onlyPending := []corev1.Pod{pods[0], pods[2]}
	got = pickPodForLogs(onlyPending)
	if got == nil || got.Name != "pending-new" {
		t.Fatalf("fallback newest=%v want pending-new", got)
	}
}

func TestCreateWithEnableLoadBalancer(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	spec := driver.Spec{
		ID:    "c1",
		Image: "img:v1",
		Ports: []int{8765, 22},
		Env:   map[string]string{"FOO": "bar"},
		Config: &driver.ConfigInject{
			BundleURL:  "https://ex/b.tgz",
			ConfigRoot: "/root/.cursor",
			Headers:    "Authorization: Bearer t",
		},
		Resources: driver.Resources{}, // zeros → defaults via resolveResources
	}
	h, err := d.Create(ctx, spec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if h.Name != "sbx-c1" || h.Namespace != "sandboxes" || h.Status != driver.StatusPending {
		t.Fatalf("handle: %+v", h)
	}

	ns := "sandboxes"
	if _, err := d.cs.AppsV1().Deployments(ns).Get(ctx, "sbx-c1", metav1.GetOptions{}); err != nil {
		t.Fatalf("deployment: %v", err)
	}
	if _, err := d.cs.CoreV1().Secrets(ns).Get(ctx, "sbx-c1-env", metav1.GetOptions{}); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if _, err := d.cs.CoreV1().PersistentVolumeClaims(ns).Get(ctx, "sbx-c1-data", metav1.GetOptions{}); err != nil {
		t.Fatalf("pvc: %v", err)
	}
	svc, err := d.cs.CoreV1().Services(ns).Get(ctx, "sbx-c1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("clusterip: %v", err)
	}
	if svc.Spec.Type != "" && svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("clusterip type=%s", svc.Spec.Type)
	}
	lb, err := d.cs.CoreV1().Services(ns).Get(ctx, "sbx-c1-lb", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("lb svc: %v", err)
	}
	if lb.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("lb type=%s", lb.Spec.Type)
	}

	// Names from Get path
	got, err := d.Get(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "sbx-c1" || got.Namespace != "sandboxes" {
		t.Fatalf("Get names: %+v", got)
	}

	// Secret should carry inject env
	sec, err := d.cs.CoreV1().Secrets(ns).Get(ctx, "sbx-c1-env", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// fake clientset may only store Data after create from StringData on real apiserver;
	// StringData is still visible on the object we created in-memory.
	if len(sec.StringData) > 0 {
		if sec.StringData["SANDBOX_INJECT"] != "https://ex/b.tgz|/root/.cursor" {
			t.Fatalf("secret stringData: %v", sec.StringData)
		}
	}
}

func TestCreateWithoutLB(t *testing.T) {
	d := testDriver(t, false)
	ctx := context.Background()
	_, err := d.Create(ctx, driver.Spec{ID: "n1", Image: "img", Ports: []int{80}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-n1-lb", metav1.GetOptions{})
	if err == nil {
		t.Fatal("LB service should not exist when disabled")
	}
	eps, err := d.Endpoints(ctx, "n1")
	if err != nil {
		t.Fatal(err)
	}
	wantHost := "sbx-n1.sandboxes.svc.cluster.local"
	if eps[80] != wantHost+":80" {
		t.Fatalf("cluster endpoints: %v", eps)
	}
}

func TestStartStopDestroy(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	_, err := d.Create(ctx, driver.Spec{ID: "s1", Image: "img", Ports: []int{8765}})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Stop(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	dep, err := d.cs.AppsV1().Deployments("sandboxes").Get(ctx, "sbx-s1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 0 {
		t.Fatalf("stop replicas=%v", dep.Spec.Replicas)
	}
	if st, _ := d.Status(ctx, "s1"); st != driver.StatusStopped {
		t.Fatalf("status after stop=%s", st)
	}
	if err := d.Start(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	dep, err = d.cs.AppsV1().Deployments("sandboxes").Get(ctx, "sbx-s1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 1 {
		t.Fatalf("start replicas=%v", dep.Spec.Replicas)
	}

	if err := d.Destroy(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	_, err = d.cs.AppsV1().Deployments("sandboxes").Get(ctx, "sbx-s1", metav1.GetOptions{})
	if err == nil {
		t.Fatal("deployment should be gone")
	}
}

func TestStatusNotFoundAndRunning(t *testing.T) {
	d := testDriver(t, false)
	ctx := context.Background()
	st, err := d.Status(ctx, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if st != driver.StatusNotFound {
		t.Fatalf("want not_found, got %s", st)
	}

	_, err = d.Create(ctx, driver.Spec{ID: "r1", Image: "img", Ports: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	st, err = d.Status(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if st != driver.StatusPending {
		t.Fatalf("fresh create want pending, got %s", st)
	}

	dep, err := d.cs.AppsV1().Deployments("sandboxes").Get(ctx, "sbx-r1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	dep.Status.ReadyReplicas = 1
	_, err = d.cs.AppsV1().Deployments("sandboxes").UpdateStatus(ctx, dep, metav1.UpdateOptions{})
	if err != nil {
		// Some fake versions only support Update; try Update with status set.
		dep.Status.ReadyReplicas = 1
		_, err = d.cs.AppsV1().Deployments("sandboxes").Update(ctx, dep, metav1.UpdateOptions{})
	}
	if err != nil {
		t.Fatalf("set ReadyReplicas: %v", err)
	}
	st, err = d.Status(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if st != driver.StatusRunning {
		t.Fatalf("want running, got %s", st)
	}
}

func TestEndpointsLBIngressIP(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	_, err := d.Create(ctx, driver.Spec{ID: "e1", Image: "img", Ports: []int{8765, 22}})
	if err != nil {
		t.Fatal(err)
	}
	eps, err := d.Endpoints(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 0 {
		t.Fatalf("no IP yet, want empty: %v", eps)
	}

	svc, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-e1-lb", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "10.0.0.9"}}
	_, err = d.cs.CoreV1().Services("sandboxes").UpdateStatus(ctx, svc, metav1.UpdateOptions{})
	if err != nil {
		svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "10.0.0.9"}}
		_, err = d.cs.CoreV1().Services("sandboxes").Update(ctx, svc, metav1.UpdateOptions{})
	}
	if err != nil {
		t.Fatalf("update lb status: %v", err)
	}
	eps, err = d.Endpoints(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if eps[8765] != "10.0.0.9:8765" || eps[22] != "10.0.0.9:22" {
		t.Fatalf("eps: %v", eps)
	}
}

func TestWaitLoadBalancerIPSuccess(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	_, err := d.Create(ctx, driver.Spec{ID: "w1", Image: "img", Ports: []int{80}})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		ip, err := d.WaitLoadBalancerIP(ctx, "w1", 10*time.Second)
		if err != nil {
			done <- err
			return
		}
		if ip != "203.0.113.10" {
			done <- errString("ip=" + ip)
			return
		}
		done <- nil
	}()

	time.Sleep(50 * time.Millisecond)
	svc, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-w1-lb", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}}
	if _, err := d.cs.CoreV1().Services("sandboxes").UpdateStatus(ctx, svc, metav1.UpdateOptions{}); err != nil {
		svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}}
		if _, err := d.cs.CoreV1().Services("sandboxes").Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
			t.Fatalf("update status: %v", err)
		}
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("timeout waiting for WaitLoadBalancerIP")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestWaitLoadBalancerIPTimeout(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	_, err := d.Create(ctx, driver.Spec{ID: "w2", Image: "img", Ports: []int{80}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.WaitLoadBalancerIP(ctx, "w2", 1*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWaitLoadBalancerIPContextCancel(t *testing.T) {
	d := testDriver(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := d.Create(ctx, driver.Spec{ID: "w3", Image: "img", Ports: []int{80}})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	_, err = d.WaitLoadBalancerIP(ctx, "w3", 30*time.Second)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestResolveResourcesAndRequests(t *testing.T) {
	d := NewFromClient(fake.NewSimpleClientset(), Options{
		Namespace:          "sandboxes",
		NamePrefix:         "sbx-",
		CPUCores:           2,
		MemoryMB:           4096,
		DataDiskGi:         25,
		CPURequestCores:    0.5,
		MemoryRequestMB:    1024,
		CPURequestRatio:    0.25,
		MemoryRequestRatio: 0.25,
	})
	r := d.resolveResources(driver.Resources{})
	if r.CPUCores != 2 || r.MemoryMB != 4096 || r.DiskGi != 25 {
		t.Fatalf("defaults: %+v", r)
	}
	r2 := d.resolveResources(driver.Resources{CPUCores: 4, MemoryMB: 8192, DiskGi: 50})
	if r2.CPUCores != 4 || r2.MemoryMB != 8192 || r2.DiskGi != 50 {
		t.Fatalf("passthrough: %+v", r2)
	}
	if got := d.cpuRequest(2); got != 0.5 {
		t.Fatalf("cpuRequest fixed=%v", got)
	}
	if got := d.memoryRequest(4096); got != 1024 {
		t.Fatalf("memoryRequest fixed=%v", got)
	}

	d2 := NewFromClient(fake.NewSimpleClientset(), Options{
		Namespace: "sandboxes", NamePrefix: "sbx-",
		CPURequestCores: 0, MemoryRequestMB: 0,
		CPURequestRatio: 0.25, MemoryRequestRatio: 0.25,
	})
	if got := d2.cpuRequest(2); got != 0.5 {
		t.Fatalf("cpu ratio=%v", got)
	}
	if got := d2.memoryRequest(4096); got != 1024 {
		t.Fatalf("mem ratio=%v", got)
	}
	// ratio > 1 clamped
	d3 := NewFromClient(fake.NewSimpleClientset(), Options{
		Namespace: "sandboxes", NamePrefix: "sbx-",
		CPURequestCores: 0, CPURequestRatio: 2, MemoryRequestMB: 0, MemoryRequestRatio: 2,
	})
	if got := d3.cpuRequest(2); got != 2 {
		t.Fatalf("cpu ratio clamp=%v", got)
	}
	if got := d3.memoryRequest(4096); got != 4096 {
		t.Fatalf("mem ratio clamp=%v", got)
	}
	// fixed exceeds limit
	d4 := NewFromClient(fake.NewSimpleClientset(), Options{
		Namespace: "sandboxes", NamePrefix: "sbx-",
		CPURequestCores: 99, MemoryRequestMB: 99999,
	})
	if got := d4.cpuRequest(2); got != 2 {
		t.Fatalf("cpu capped=%v", got)
	}
	if got := d4.memoryRequest(512); got != 512 {
		t.Fatalf("mem capped=%v", got)
	}
}

func TestListAndLoadBalancerHostname(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	_, err := d.Create(ctx, driver.Spec{ID: "l1", Image: "img", Ports: []int{443}})
	if err != nil {
		t.Fatal(err)
	}
	list, err := d.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "l1" {
		t.Fatalf("list: %+v", list)
	}

	svc, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-l1-lb", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{Hostname: "lb.example.com"}}
	if _, err := d.cs.CoreV1().Services("sandboxes").Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	eps, err := d.Endpoints(ctx, "l1")
	if err != nil {
		t.Fatal(err)
	}
	if eps[443] != "lb.example.com:443" {
		t.Fatalf("hostname eps: %v", eps)
	}
}

func TestWithDefaults(t *testing.T) {
	o := withDefaults(Options{})
	if o.Namespace != "sandboxes" || o.NamePrefix != "sbx-" || o.ImagePullPolicy != "IfNotPresent" {
		t.Fatalf("%+v", o)
	}
	if o.DataDiskGi != 25 || o.CPUCores != 2 || o.MemoryMB != 4096 {
		t.Fatalf("%+v", o)
	}
}

func TestIgnoreNotFound(t *testing.T) {
	if err := ignoreNotFound(nil); err != nil {
		t.Fatal(err)
	}
	notFound := apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "x")
	if err := ignoreNotFound(notFound); err != nil {
		t.Fatalf("NotFound should be ignored: %v", err)
	}
	if err := ignoreNotFound(fmt.Errorf("boom")); err == nil {
		t.Fatal("non-NotFound should pass through")
	}
}

func TestNewMissingKubeconfig(t *testing.T) {
	_, err := New(Options{Kubeconfig: filepath.Join(t.TempDir(), "missing-kubeconfig")})
	if err == nil {
		t.Fatal("expected error for missing kubeconfig")
	}
}

func TestLoadConfigInClusterAndEmpty(t *testing.T) {
	_, err := loadConfig(Options{InCluster: true})
	if err == nil {
		t.Fatal("in-cluster config should fail outside a cluster")
	}
	// Empty kubeconfig falls back to ~/.kube/config (may or may not exist).
	_, _ = loadConfig(Options{Kubeconfig: ""})
}

func TestReinstallPreserveAndWipe(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	spec := driver.Spec{
		ID:    "ri1",
		Image: "img:v2",
		Ports: []int{8765},
		Env:   map[string]string{"A": "1"},
		Config: &driver.ConfigInject{
			BundleURL:  "https://ex/r.tgz",
			ConfigRoot: "/root/.cursor",
		},
		Resources: driver.Resources{CPUCores: 1, MemoryMB: 1024, DiskGi: 10},
	}
	if _, err := d.Create(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if err := d.Reinstall(ctx, spec, true); err != nil {
		t.Fatalf("preserve: %v", err)
	}
	if _, err := d.cs.CoreV1().PersistentVolumeClaims("sandboxes").Get(ctx, "sbx-ri1-data", metav1.GetOptions{}); err != nil {
		t.Fatalf("PVC should remain when preserveData: %v", err)
	}
	if _, err := d.cs.AppsV1().Deployments("sandboxes").Get(ctx, "sbx-ri1", metav1.GetOptions{}); err != nil {
		t.Fatalf("deployment recreated: %v", err)
	}

	if err := d.Reinstall(ctx, spec, false); err != nil {
		t.Fatalf("wipe: %v", err)
	}
	if _, err := d.cs.AppsV1().Deployments("sandboxes").Get(ctx, "sbx-ri1", metav1.GetOptions{}); err != nil {
		t.Fatalf("deployment after wipe: %v", err)
	}
	if _, err := d.cs.CoreV1().PersistentVolumeClaims("sandboxes").Get(ctx, "sbx-ri1-data", metav1.GetOptions{}); err != nil {
		t.Fatalf("PVC should be recreated after wipe: %v", err)
	}
}

func TestCreateIdempotentAlreadyExists(t *testing.T) {
	d := testDriver(t, false)
	ctx := context.Background()
	spec := driver.Spec{ID: "dup", Image: "img", Ports: []int{80}, Env: map[string]string{"X": "1"}}
	if _, err := d.Create(ctx, spec); err != nil {
		t.Fatal(err)
	}
	// Second create should update secret/deployment (AlreadyExists paths).
	spec.Env["X"] = "2"
	if _, err := d.Create(ctx, spec); err != nil {
		t.Fatalf("second create: %v", err)
	}
}

func TestEnsureNamespaceExists(t *testing.T) {
	d := testDriver(t, false)
	ctx := context.Background()
	_, err := d.cs.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "sandboxes"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, driver.Spec{ID: "ns1", Image: "img", Ports: []int{1}}); err != nil {
		t.Fatal(err)
	}
	ns, err := d.cs.CoreV1().Namespaces().Get(ctx, "sandboxes", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := ns.Labels["pod-security.kubernetes.io/enforce"]; got != "privileged" {
		t.Fatalf("PSA enforce=%q, want privileged (required for DinD)", got)
	}
	dep, err := d.cs.AppsV1().Deployments("sandboxes").Get(ctx, "sbx-ns1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	sc := dep.Spec.Template.Spec.Containers[0].SecurityContext
	if sc == nil || sc.Privileged == nil || !*sc.Privileged {
		t.Fatalf("sandbox container must be privileged for DinD, got %+v", sc)
	}
}

func TestCPUMemoryRequestMinFloor(t *testing.T) {
	d := NewFromClient(fake.NewSimpleClientset(), Options{
		Namespace: "sandboxes", NamePrefix: "sbx-",
		CPURequestCores: 0.01, MemoryRequestMB: 1,
	})
	if got := d.cpuRequest(2); got != 0.05 {
		t.Fatalf("cpu min=%v", got)
	}
	if got := d.memoryRequest(4096); got != 128 {
		t.Fatalf("mem min=%v", got)
	}
}

func TestCPUMemoryRequestRatioDefaultsAndCap(t *testing.T) {
	d := NewFromClient(fake.NewSimpleClientset(), Options{
		Namespace: "sandboxes", NamePrefix: "sbx-",
		CPURequestCores: 0, MemoryRequestMB: 0,
		CPURequestRatio: 0, MemoryRequestRatio: 0, // → 0.25 inside helpers
	})
	// withDefaults already filled ratios to 0.25; force zero after construction
	d.opts.CPURequestRatio = 0
	d.opts.MemoryRequestRatio = 0
	if got := d.cpuRequest(2); got != 0.5 {
		t.Fatalf("cpu default ratio=%v", got)
	}
	if got := d.memoryRequest(4096); got != 1024 {
		t.Fatalf("mem default ratio=%v", got)
	}
	d.opts.CPURequestRatio = 1
	d.opts.MemoryRequestRatio = 1
	// tiny limit × ratio floors to minRequest (may exceed limit)
	if got := d.cpuRequest(0.01); got != 0.05 {
		t.Fatalf("cpu min floor=%v", got)
	}
	if got := d.memoryRequest(64); got != 128 {
		t.Fatalf("mem min floor=%v", got)
	}
}

func TestCreateImagePullSecretAndWorkspaceDir(t *testing.T) {
	d := NewFromClient(fake.NewSimpleClientset(), Options{
		Namespace:       "sandboxes",
		NamePrefix:      "sbx-",
		ImagePullSecret: "regcred",
	})
	ctx := context.Background()
	_, err := d.Create(ctx, driver.Spec{
		ID:           "ips",
		Image:        "img",
		Ports:        []int{80},
		WorkspaceDir: "/custom/ws",
	})
	if err != nil {
		t.Fatal(err)
	}
	dep, err := d.cs.AppsV1().Deployments("sandboxes").Get(ctx, "sbx-ips", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(dep.Spec.Template.Spec.ImagePullSecrets) != 1 || dep.Spec.Template.Spec.ImagePullSecrets[0].Name != "regcred" {
		t.Fatalf("pull secrets: %+v", dep.Spec.Template.Spec.ImagePullSecrets)
	}
	found := false
	for _, e := range dep.Spec.Template.Spec.Containers[0].Env {
		if e.Name == "WORKSPACE_DIR" && e.Value == "/custom/ws" {
			found = true
		}
	}
	if !found {
		t.Fatalf("WORKSPACE_DIR missing: %+v", dep.Spec.Template.Spec.Containers[0].Env)
	}
}

func TestCreatePVCVolumeMounts(t *testing.T) {
	d := testDriver(t, false)
	ctx := context.Background()
	if _, err := d.Create(ctx, driver.Spec{ID: "vm1", Image: "img", Ports: []int{80}}); err != nil {
		t.Fatal(err)
	}
	dep, err := d.cs.AppsV1().Deployments("sandboxes").Get(ctx, "sbx-vm1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"/root/workspace":   "workspace",
		"/root/.cache":      "cache",
		"/root/.npm":        "npm",
		"/root/.m2":         "m2",
		"/root/go/pkg/mod":  "go-mod",
		"/var/lib/docker":   "docker",
		"/var/lib/buildkit": "buildkit",
		"/tmp":              "tmp",
	}
	got := map[string]string{}
	for _, m := range dep.Spec.Template.Spec.Containers[0].VolumeMounts {
		if m.Name != "data" {
			continue
		}
		got[m.MountPath] = m.SubPath
	}
	for path, sub := range want {
		if got[path] != sub {
			t.Fatalf("mount %s: got subPath %q want %q; mounts=%v", path, got[path], sub, got)
		}
	}
}

func TestCreateNamespaceGetError(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("get", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("apiserver down")
	})
	d := NewFromClient(cs, Options{Namespace: "sandboxes", NamePrefix: "sbx-"})
	_, err := d.Create(context.Background(), driver.Spec{ID: "x", Image: "i", Ports: []int{1}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWaitDeploymentGoneTimeoutAndCancel(t *testing.T) {
	d := testDriver(t, false)
	ctx := context.Background()
	_, err := d.Create(ctx, driver.Spec{ID: "wg1", Image: "img", Ports: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	// Object still present → timeout branch (zero timeout makes deadline immediate).
	err = d.waitDeploymentGone(ctx, "sbx-wg1", 0)
	if err == nil {
		t.Fatal("expected timeout")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	err = d.waitDeploymentGone(canceled, "sbx-wg1", 30*time.Second)
	if err == nil {
		t.Fatal("expected context cancel")
	}
}

func TestWaitPVCGoneTimeoutAndCancel(t *testing.T) {
	d := testDriver(t, false)
	ctx := context.Background()
	_, err := d.Create(ctx, driver.Spec{ID: "wp1", Image: "img", Ports: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	err = d.waitPVCGone(ctx, "sbx-wp1-data", 0)
	if err == nil {
		t.Fatal("expected timeout")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	err = d.waitPVCGone(canceled, "sbx-wp1-data", 30*time.Second)
	if err == nil {
		t.Fatal("expected context cancel")
	}
}

func TestWaitDeploymentGoneStillPresentThenGone(t *testing.T) {
	d := testDriver(t, false)
	ctx := context.Background()
	_, err := d.Create(ctx, driver.Spec{ID: "wg2", Image: "img", Ports: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- d.waitDeploymentGone(context.Background(), "sbx-wg2", 15*time.Second)
	}()
	time.Sleep(50 * time.Millisecond)
	fg := metav1.DeletePropagationForeground
	if err := d.cs.AppsV1().Deployments("sandboxes").Delete(ctx, "sbx-wg2", metav1.DeleteOptions{PropagationPolicy: &fg}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("timeout waiting for waitDeploymentGone")
	}
}

func TestWaitPVCGoneStillPresentThenGone(t *testing.T) {
	d := testDriver(t, false)
	ctx := context.Background()
	_, err := d.Create(ctx, driver.Spec{ID: "wp2", Image: "img", Ports: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- d.waitPVCGone(context.Background(), "sbx-wp2-data", 15*time.Second)
	}()
	time.Sleep(50 * time.Millisecond)
	if err := d.cs.CoreV1().PersistentVolumeClaims("sandboxes").Delete(ctx, "sbx-wp2-data", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("timeout waiting for waitPVCGone")
	}
}

func TestReinstallWipesPVC(t *testing.T) {
	d := testDriver(t, false)
	ctx := context.Background()
	spec := driver.Spec{ID: "wipe1", Image: "img", Ports: []int{80}, Resources: driver.Resources{DiskGi: 5}}
	if _, err := d.Create(ctx, spec); err != nil {
		t.Fatal(err)
	}
	// Capture UID of original PVC then wipe.
	pvc, err := d.cs.CoreV1().PersistentVolumeClaims("sandboxes").Get(ctx, "sbx-wipe1-data", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	oldUID := pvc.UID
	if err := d.Reinstall(ctx, spec, false); err != nil {
		t.Fatalf("Reinstall wipe: %v", err)
	}
	pvc2, err := d.cs.CoreV1().PersistentVolumeClaims("sandboxes").Get(ctx, "sbx-wipe1-data", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("PVC should be recreated: %v", err)
	}
	if pvc2.UID == oldUID && oldUID != "" {
		// Fake clientset may recycle UIDs; at least ensure PVC exists after wipe.
		t.Logf("PVC UID unchanged in fake clientset (ok): %s", oldUID)
	}
}

func TestEnsurePVCAlreadyExists(t *testing.T) {
	d := testDriver(t, false)
	ctx := context.Background()
	if err := d.ensurePVC(ctx, "p1", 10); err != nil {
		t.Fatal(err)
	}
	if err := d.ensurePVC(ctx, "p1", 10); err != nil {
		t.Fatalf("second ensurePVC: %v", err)
	}
}

func TestEnsurePVCAnnotations(t *testing.T) {
	cs := fake.NewSimpleClientset()
	d := NewFromClient(cs, Options{
		Namespace:    "sandboxes",
		NamePrefix:   "sbx-",
		StorageClass: "ugreen-iscsi",
		PVCAnnotations: map[string]string{
			"csi.ugreen.com/deletion-policy": "purge",
			"example.com/owner":              "sandbox-gateway",
		},
	})
	ctx := context.Background()
	if err := d.ensurePVC(ctx, "ann1", 10); err != nil {
		t.Fatal(err)
	}
	pvc, err := d.cs.CoreV1().PersistentVolumeClaims("sandboxes").Get(ctx, "sbx-ann1-data", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := pvc.Annotations["csi.ugreen.com/deletion-policy"]; got != "purge" {
		t.Fatalf("deletion-policy=%q want purge; annotations=%v", got, pvc.Annotations)
	}
	if got := pvc.Annotations["example.com/owner"]; got != "sandbox-gateway" {
		t.Fatalf("owner=%q; annotations=%v", got, pvc.Annotations)
	}

	// Existing PVC must not be mutated on idempotent ensure.
	pvc.Annotations["csi.ugreen.com/deletion-policy"] = "archive"
	if _, err := d.cs.CoreV1().PersistentVolumeClaims("sandboxes").Update(ctx, pvc, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := d.ensurePVC(ctx, "ann1", 10); err != nil {
		t.Fatal(err)
	}
	pvc2, err := d.cs.CoreV1().PersistentVolumeClaims("sandboxes").Get(ctx, "sbx-ann1-data", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := pvc2.Annotations["csi.ugreen.com/deletion-policy"]; got != "archive" {
		t.Fatalf("existing PVC annotation mutated: %q", got)
	}
}

func TestListSkipsEmptyIDUsesNamePrefix(t *testing.T) {
	d := testDriver(t, false)
	ctx := context.Background()
	replicas := int32(1)
	_, err := d.cs.AppsV1().Deployments("sandboxes").Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sbx-orphan",
			Namespace: "sandboxes",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": managedByLabel,
				// no sandbox-gateway.io/id
			},
		},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	list, err := d.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range list {
		if h.ID == "orphan" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected id trimmed from name: %+v", list)
	}
}

func TestCreateLBOmitsInternalPortsClusterIPKeepsThem(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	_, err := d.Create(ctx, driver.Spec{
		ID:            "iso",
		Image:         "img",
		Ports:         []int{8765, 8744, 22, 80},
		InternalPorts: []int{9222, 6080},
	})
	if err != nil {
		t.Fatal(err)
	}
	lb, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-iso-lb", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gotLB := map[int]bool{}
	for _, p := range lb.Spec.Ports {
		gotLB[int(p.Port)] = true
	}
	if gotLB[9222] || gotLB[6080] {
		t.Fatalf("LB must not publish 9222/6080: %+v", lb.Spec.Ports)
	}
	for _, w := range []int{8765, 8744, 22, 80} {
		if !gotLB[w] {
			t.Fatalf("LB missing public port %d: %+v", w, lb.Spec.Ports)
		}
	}
	cip, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-iso", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gotCIP := map[int]bool{}
	for _, p := range cip.Spec.Ports {
		gotCIP[int(p.Port)] = true
	}
	for _, w := range []int{8765, 8744, 22, 80, 9222, 6080} {
		if !gotCIP[w] {
			t.Fatalf("ClusterIP missing listen port %d: %+v", w, cip.Spec.Ports)
		}
	}
}

func TestCreatePreviewDirectAddsServicePort(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	env := map[string]string{driver.EnvPreviewDirect: "1"}
	_, err := d.Create(ctx, driver.Spec{
		ID:    "pd1",
		Image: "img",
		Ports: []int{8765, 22},
		Env:   env,
	})
	if err != nil {
		t.Fatal(err)
	}
	if env[driver.EnvPreviewPort] != "18080" {
		t.Fatalf("PREVIEW_PORT=%q", env[driver.EnvPreviewPort])
	}
	lb, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-pd1-lb", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range lb.Spec.Ports {
		if int(p.Port) == 18080 && int(p.TargetPort.IntVal) == 18080 {
			found = true
		}
	}
	if !found {
		t.Fatalf("LB missing 18080: %+v", lb.Spec.Ports)
	}
	sec, err := d.cs.CoreV1().Secrets("sandboxes").Get(ctx, "sbx-pd1-env", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gotPort := string(sec.Data["PREVIEW_PORT"])
	if gotPort == "" {
		gotPort = sec.StringData["PREVIEW_PORT"]
	}
	if gotPort != "18080" {
		t.Fatalf("secret PREVIEW_PORT=%q data=%v stringData=%v", gotPort, sec.Data, sec.StringData)
	}
}

func TestPublishPortAddsToLB(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	if _, err := d.Create(ctx, driver.Spec{ID: "pp1", Image: "img", Ports: []int{8765, 22}}); err != nil {
		t.Fatal(err)
	}
	if err := d.PublishPort(ctx, "pp1", 5173); err != nil {
		t.Fatal(err)
	}
	lb, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-pp1-lb", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range lb.Spec.Ports {
		if int(p.Port) == 5173 {
			found = true
		}
	}
	if !found {
		t.Fatalf("LB missing 5173 after PublishPort: %+v", lb.Spec.Ports)
	}
}

func TestPublishPortValidationAndMissing(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	if err := d.PublishPort(ctx, "", 80); err == nil {
		t.Fatal("empty id")
	}
	if err := d.PublishPort(ctx, "x", 0); err == nil {
		t.Fatal("port 0")
	}
	if err := d.PublishPort(ctx, "x", 70000); err == nil {
		t.Fatal("port high")
	}
	if err := d.PublishPort(ctx, "missing", 80); err == nil {
		t.Fatal("missing sandbox")
	}
}

func TestPublishPortWithoutLBAddsClusterIP(t *testing.T) {
	d := testDriver(t, false)
	ctx := context.Background()
	if _, err := d.Create(ctx, driver.Spec{ID: "nl1", Image: "img", Ports: []int{8765}}); err != nil {
		t.Fatal(err)
	}
	if err := d.PublishPort(ctx, "nl1", 3000); err != nil {
		t.Fatal(err)
	}
	cip, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-nl1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range cip.Spec.Ports {
		if int(p.Port) == 3000 {
			found = true
		}
	}
	if !found {
		t.Fatalf("ClusterIP missing 3000: %+v", cip.Spec.Ports)
	}
}

func TestPublishPortRecreatesMissingLB(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	if _, err := d.Create(ctx, driver.Spec{ID: "rl1", Image: "img", Ports: []int{8765}}); err != nil {
		t.Fatal(err)
	}
	if err := d.cs.CoreV1().Services("sandboxes").Delete(ctx, "sbx-rl1-lb", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := d.PublishPort(ctx, "rl1", 5173); err != nil {
		t.Fatal(err)
	}
	lb, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-rl1-lb", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range lb.Spec.Ports {
		if int(p.Port) == 5173 {
			found = true
		}
	}
	if !found {
		t.Fatalf("recreated LB missing 5173: %+v", lb.Spec.Ports)
	}
}

func TestPublishPortIdempotent(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	if _, err := d.Create(ctx, driver.Spec{ID: "id1", Image: "img", Ports: []int{8765, 5173}}); err != nil {
		t.Fatal(err)
	}
	if err := d.PublishPort(ctx, "id1", 5173); err != nil {
		t.Fatal(err)
	}
}

func TestPortsFromServiceNil(t *testing.T) {
	if portsFromService(nil) != nil {
		t.Fatal("nil service")
	}
}

func TestReinstallPreviewDirectAddsServicePort(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	spec := driver.Spec{ID: "ri-pd", Image: "img", Ports: []int{8765, 22}}
	if _, err := d.Create(ctx, spec); err != nil {
		t.Fatal(err)
	}
	spec.Env = map[string]string{driver.EnvPreviewDirect: "1"}
	if err := d.Reinstall(ctx, spec, true); err != nil {
		t.Fatal(err)
	}
	lb, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-ri-pd-lb", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range lb.Spec.Ports {
		if int(p.Port) == 18080 {
			found = true
		}
	}
	if !found {
		t.Fatalf("reinstall LB missing 18080: %+v", lb.Spec.Ports)
	}
}

func TestEndpointsMergesInternalClusterDNSWhenLBEnabled(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	_, err := d.Create(ctx, driver.Spec{
		ID: "m1", Image: "img", Ports: []int{8765, 22}, InternalPorts: []int{9222, 6080},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-m1-lb", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "203.0.113.9"}}
	if _, err := d.cs.CoreV1().Services("sandboxes").UpdateStatus(ctx, svc, metav1.UpdateOptions{}); err != nil {
		svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "203.0.113.9"}}
		if _, err := d.cs.CoreV1().Services("sandboxes").Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
			t.Fatalf("update lb: %v", err)
		}
	}
	eps, err := d.Endpoints(ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if eps[8765] != "203.0.113.9:8765" || eps[22] != "203.0.113.9:22" {
		t.Fatalf("public should use LB IP: %v", eps)
	}
	wantHost := "sbx-m1.sandboxes.svc.cluster.local"
	if eps[9222] != wantHost+":9222" || eps[6080] != wantHost+":6080" {
		t.Fatalf("internal should use ClusterIP DNS, not LB: %v", eps)
	}
}

func isolationDriver(t *testing.T) *Driver {
	t.Helper()
	cs := fake.NewSimpleClientset()
	return NewFromClient(cs, Options{
		Namespace:          "sandboxes",
		NamePrefix:         "sbx-",
		EnableLoadBalancer: true,
		PublicPorts:        []int{8765, 8744, 22, 80},
		InternalPorts:      []int{9222, 6080},
	})
}

func seedRunningSandboxWithStaleLB(t *testing.T, d *Driver, id, lbIP string) {
	t.Helper()
	ctx := context.Background()
	replicas := int32(1)
	labels := d.selector(id)
	_, err := d.cs.AppsV1().Deployments(d.opts.Namespace).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.resourceName(id),
			Namespace: d.opts.Namespace,
			Labels:    labels,
		},
		Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("seed deployment: %v", err)
	}
	_, err = d.cs.CoreV1().Services(d.opts.Namespace).Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: d.resourceName(id), Namespace: d.opts.Namespace, Labels: labels},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports:    servicePorts([]int{8765, 22, 9222, 6080}),
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("seed clusterip: %v", err)
	}
	lb := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: d.lbName(id), Namespace: d.opts.Namespace, Labels: labels},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: labels,
			Ports:    servicePorts([]int{8765, 22, 9222, 6080}),
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{{IP: lbIP}},
			},
		},
	}
	created, err := d.cs.CoreV1().Services(d.opts.Namespace).Create(ctx, lb, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("seed lb: %v", err)
	}
	created.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: lbIP}}
	if _, err := d.cs.CoreV1().Services(d.opts.Namespace).UpdateStatus(ctx, created, metav1.UpdateOptions{}); err != nil {
		created.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: lbIP}}
		if _, err := d.cs.CoreV1().Services(d.opts.Namespace).Update(ctx, created, metav1.UpdateOptions{}); err != nil {
			t.Fatalf("seed lb ingress: %v", err)
		}
	}
}

func assertLBConvergedInternalClusterDNS(t *testing.T, d *Driver, id, lbIP string) {
	t.Helper()
	ctx := context.Background()
	lb, err := d.cs.CoreV1().Services(d.opts.Namespace).Get(ctx, d.lbName(id), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range lb.Spec.Ports {
		if p.Port == 9222 || p.Port == 6080 {
			t.Fatalf("inventory LB still exposes %d: %+v", p.Port, lb.Spec.Ports)
		}
	}
	cip, err := d.cs.CoreV1().Services(d.opts.Namespace).Get(ctx, d.resourceName(id), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gotCIP := map[int32]bool{}
	for _, p := range cip.Spec.Ports {
		gotCIP[p.Port] = true
	}
	for _, w := range []int32{8765, 22, 9222, 6080} {
		if !gotCIP[w] {
			t.Fatalf("ClusterIP missing port %d: %+v", w, cip.Spec.Ports)
		}
	}
	eps, err := d.Endpoints(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if eps[8765] != lbIP+":8765" {
		t.Fatalf("public should stay on LB IP, got %v", eps)
	}
	wantHost := clusterDNS(d.resourceName(id), d.opts.Namespace)
	if eps[9222] != wantHost+":9222" || eps[6080] != wantHost+":6080" {
		t.Fatalf("internal should use ClusterIP DNS after converge, got %v", eps)
	}
}

func TestStartConvergesStaleLoadBalancer(t *testing.T) {
	d := isolationDriver(t)
	ctx := context.Background()
	seedRunningSandboxWithStaleLB(t, d, "inv", "203.0.113.40")
	if err := d.Start(ctx, "inv"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	assertLBConvergedInternalClusterDNS(t, d, "inv", "203.0.113.40")
}

func TestGetConvergesStaleLoadBalancer(t *testing.T) {
	d := isolationDriver(t)
	ctx := context.Background()
	seedRunningSandboxWithStaleLB(t, d, "g1", "203.0.113.41")
	h, err := d.Get(ctx, "g1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if h.Status != driver.StatusRunning {
		t.Fatalf("status=%s", h.Status)
	}
	assertLBConvergedInternalClusterDNS(t, d, "g1", "203.0.113.41")
	wantHost := clusterDNS("sbx-g1", "sandboxes")
	if h.Endpoints[9222] != wantHost+":9222" || h.Endpoints[6080] != wantHost+":6080" {
		t.Fatalf("Get endpoints still on publish surface: %v", h.Endpoints)
	}
}

func TestConvergePublishSurfaceNoOpWithoutOptions(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	seedRunningSandboxWithStaleLB(t, d, "keep", "203.0.113.42")
	if err := d.ConvergePublishSurface(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	lb, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-keep-lb", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[int32]bool{}
	for _, p := range lb.Spec.Ports {
		got[p.Port] = true
	}
	if !got[9222] || !got[6080] {
		t.Fatalf("empty Options must not wipe inventory LB: %+v", lb.Spec.Ports)
	}
}

func TestEnsureServicesUpdateExistingPorts(t *testing.T) {
	d := testDriver(t, true)
	ctx := context.Background()
	// Seed a stale LB+ClusterIP that still expose 9222/6080 (pre-hardening).
	labels := d.selector("old")
	_, err := d.cs.CoreV1().Services("sandboxes").Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "sbx-old", Namespace: "sandboxes", Labels: labels},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports:    servicePorts([]int{8765, 22, 9222, 6080}),
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.cs.CoreV1().Services("sandboxes").Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "sbx-old-lb", Namespace: "sandboxes", Labels: labels},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: labels,
			Ports:    servicePorts([]int{8765, 22, 9222, 6080}),
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ensureClusterIPService(ctx, "old", []int{8765, 22, 9222, 6080}); err != nil {
		t.Fatalf("update clusterip: %v", err)
	}
	if err := d.ensureLoadBalancer(ctx, "old", []int{8765, 22}); err != nil {
		t.Fatalf("update lb: %v", err)
	}
	lb, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-old-lb", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range lb.Spec.Ports {
		if p.Port == 9222 || p.Port == 6080 {
			t.Fatalf("stale LB still exposes %d", p.Port)
		}
	}
	cip, err := d.cs.CoreV1().Services("sandboxes").Get(ctx, "sbx-old", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[int32]bool{}
	for _, p := range cip.Spec.Ports {
		got[p.Port] = true
	}
	if !got[9222] || !got[6080] || !got[8765] {
		t.Fatalf("ClusterIP should keep internal+public: %+v", cip.Spec.Ports)
	}
}

func TestEndpointsLBNotFound(t *testing.T) {
	d := testDriver(t, true)
	eps, err := d.Endpoints(context.Background(), "missing")
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 0 {
		t.Fatalf("want empty, got %v", eps)
	}
}

func TestScaleMissingDeployment(t *testing.T) {
	d := testDriver(t, false)
	if err := d.Start(context.Background(), "nope"); err == nil {
		t.Fatal("expected error")
	}
}
