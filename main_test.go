/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package main

import (
	"context"
	"io/ioutil"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"k8s.io/kube-state-metrics/pkg/options"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	kcollectors "k8s.io/kube-state-metrics/pkg/collectors"
	"k8s.io/kube-state-metrics/pkg/whiteblacklist"
)

func BenchmarkKubeStateMetrics(b *testing.B) {
	var collectors []*kcollectors.Collector
	fixtureMultiplier := 1000
	requestCount := 1000

	b.Logf(
		"starting kube-state-metrics benchmark with fixtureMultiplier %v and requestCount %v",
		fixtureMultiplier,
		requestCount,
	)

	kubeClient := fake.NewSimpleClientset()

	if err := injectFixtures(kubeClient, fixtureMultiplier); err != nil {
		b.Errorf("error injecting resources: %v", err)
	}

	opts := options.NewOptions()

	builder := kcollectors.NewBuilder(context.TODO(), opts)
	builder.WithEnabledCollectors(options.DefaultCollectors)
	builder.WithKubeClient(kubeClient)
	builder.WithNamespaces(options.DefaultNamespaces)

	l, err := whiteblacklist.New(map[string]struct{}{}, map[string]struct{}{})
	if err != nil {
		b.Fatal(err)
	}
	builder.WithWhiteBlackList(l)

	// This test is not suitable to be compared in terms of time, as it includes
	// a one second wait. Use for memory allocation comparisons, profiling, ...
	b.Run("GenerateMetrics", func(b *testing.B) {
		collectors = builder.Build()

		// Wait for caches to fill
		time.Sleep(time.Second)
	})

	handler := metricHandler{collectors, false}
	req := httptest.NewRequest("GET", "http://localhost:8080/metrics", nil)

	b.Run("MakeRequests", func(b *testing.B) {
		var accumulatedContentLength int64

		for i := 0; i < requestCount; i++ {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != 200 {
				b.Fatalf("expected 200 status code but got %v", resp.StatusCode)
			}

			if resp.ContentLength == -1 {
				b.Fatal("expected content length of response not to be unknown")
			}
			accumulatedContentLength += resp.ContentLength
		}

		b.SetBytes(accumulatedContentLength)
	})
}

// TestFullScrapeCycle is a simple smoke test covering the entire cycle from
// cache filling to scraping.
func TestFullScrapeCycle(t *testing.T) {
	t.Parallel()

	kubeClient := fake.NewSimpleClientset()

	err := service(kubeClient, 0)
	if err != nil {
		t.Fatalf("failed to insert sample pod %v", err.Error())
	}

	opts := options.NewOptions()

	builder := kcollectors.NewBuilder(context.TODO(), opts)
	builder.WithEnabledCollectors(options.DefaultCollectors)
	builder.WithKubeClient(kubeClient)
	builder.WithNamespaces(options.DefaultNamespaces)

	l, err := whiteblacklist.New(map[string]struct{}{}, map[string]struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	builder.WithWhiteBlackList(l)

	collectors := builder.Build()

	// Wait for caches to fill
	time.Sleep(time.Second)

	handler := metricHandler{collectors, false}
	req := httptest.NewRequest("GET", "http://localhost:8080/metrics", nil)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 status code but got %v", resp.StatusCode)
	}

	body, _ := ioutil.ReadAll(resp.Body)

	expected := `# HELP kube_pod_info Information about pod.
# HELP kube_pod_start_time Start time in unix timestamp for a pod.
# HELP kube_pod_completion_time Completion time in unix timestamp for a pod.
# HELP kube_pod_owner Information about the Pod's owner.
# HELP kube_pod_labels Kubernetes labels converted to Prometheus labels.
# HELP kube_pod_created Unix creation timestamp
# HELP kube_pod_status_scheduled_time Unix timestamp when pod moved into scheduled status
# HELP kube_pod_status_phase The pods current phase.
# HELP kube_pod_status_ready Describes whether the pod is ready to serve requests.
# HELP kube_pod_status_scheduled Describes the status of the scheduling process for the pod.
# HELP kube_pod_container_info Information about a container in a pod.
# HELP kube_pod_container_status_waiting Describes whether the container is currently in waiting state.
# HELP kube_pod_container_status_waiting_reason Describes the reason the container is currently in waiting state.
# HELP kube_pod_container_status_running Describes whether the container is currently in running state.
# HELP kube_pod_container_status_terminated Describes whether the container is currently in terminated state.
# HELP kube_pod_container_status_terminated_reason Describes the reason the container is currently in terminated state.
# HELP kube_pod_container_status_last_terminated_reason Describes the last reason the container was in terminated state.
# HELP kube_pod_container_status_ready Describes whether the containers readiness check succeeded.
# HELP kube_pod_container_status_restarts_total The number of container restarts per container.
# HELP kube_pod_container_resource_requests The number of requested request resource by a container.
# HELP kube_pod_container_resource_limits The number of requested limit resource by a container.
# HELP kube_pod_container_resource_requests_cpu_cores The number of requested cpu cores by a container.
# HELP kube_pod_container_resource_requests_memory_bytes The number of requested memory bytes by a container.
# HELP kube_pod_container_resource_limits_cpu_cores The limit on cpu cores to be used by a container.
# HELP kube_pod_container_resource_limits_memory_bytes The limit on memory to be used by a container in bytes.
# HELP kube_pod_spec_volumes_persistentvolumeclaims_info Information about persistentvolumeclaim volumes in a pod.
# HELP kube_pod_spec_volumes_persistentvolumeclaims_readonly Describes whether a persistentvolumeclaim is mounted read only.
# HELP kube_service_info Information about service.
kube_service_info{namespace="default",service="service0",cluster_ip="",external_name="",load_balancer_ip=""} 1
# HELP kube_service_created Unix creation timestamp
# HELP kube_service_spec_type Type about service.
kube_service_spec_type{namespace="default",service="service0",type=""} 1
# HELP kube_service_labels Kubernetes labels converted to Prometheus labels.
kube_service_labels{namespace="default",service="service0"} 1
# HELP kube_service_spec_external_ip Service external ips. One series for each ip
# HELP kube_service_status_load_balancer_ingress Service load balancer ingress status`

	got := strings.TrimSpace(string(body))

	if expected != got {
		t.Fatalf("expected:\n%v\nbut got:\n%v", expected, got)
	}
}

func injectFixtures(client *fake.Clientset, multiplier int) error {
	creators := []func(*fake.Clientset, int) error{
		configMap,
		service,
		pod,
	}

	for _, c := range creators {
		for i := 0; i < multiplier; i++ {
			err := c(client, i)

			if err != nil {
				return err
			}
		}
	}

	return nil
}

func configMap(client *fake.Clientset, index int) error {
	i := strconv.Itoa(index)

	configMap := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "configmap" + i,
			ResourceVersion: "123456",
		},
	}
	_, err := client.CoreV1().ConfigMaps(metav1.NamespaceDefault).Create(&configMap)
	return err
}

func service(client *fake.Clientset, index int) error {
	i := strconv.Itoa(index)

	service := v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "service" + i,
			ResourceVersion: "123456",
		},
	}
	_, err := client.CoreV1().Services(metav1.NamespaceDefault).Create(&service)
	return err
}

func pod(client *fake.Clientset, index int) error {
	i := strconv.Itoa(index)

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod" + i,
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				v1.ContainerStatus{
					Name:        "container1",
					Image:       "k8s.gcr.io/hyperkube1",
					ImageID:     "docker://sha256:aaa",
					ContainerID: "docker://ab123",
				},
			},
		},
	}

	_, err := client.CoreV1().Pods(metav1.NamespaceDefault).Create(&pod)
	return err
}