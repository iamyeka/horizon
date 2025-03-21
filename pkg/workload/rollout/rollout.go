// Copyright © 2023 Horizoncd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rollout

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	rolloutsv1alpha1 "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	herrors "github.com/horizoncd/horizon/core/errors"
	perror "github.com/horizoncd/horizon/pkg/errors"
	"github.com/horizoncd/horizon/pkg/util/kube"
	"github.com/horizoncd/horizon/pkg/util/log"
	"github.com/horizoncd/horizon/pkg/workload"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/kubectl/pkg/polymorphichelpers"
)

var (
	GVRRollout = schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "rollouts",
	}
	GVRReplicaSet = schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "replicasets",
	}
	GVRPod = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}
)

func init() {
	workload.Register(ability, GVRRollout, GVRReplicaSet, GVRPod)
}

// please refer to github.com/horizoncd/horizon/pkg/cluster/cd/workload/workload.go
var ability = &rollout{}

type rollout struct{}

func (*rollout) MatchGK(gk schema.GroupKind) bool {
	return gk.Group == "argoproj.io" && gk.Kind == "Rollout"
}

func (*rollout) getRollout(node *v1alpha1.ResourceNode,
	rolloutInformer informers.GenericInformer) (*rolloutsv1alpha1.Rollout, *unstructured.Unstructured, error) {
	obj, err := rolloutInformer.Lister().ByNamespace(node.Namespace).Get(node.Name)
	if err != nil {
		return nil, nil, perror.Wrapf(
			herrors.NewErrGetFailed(herrors.ResourceInK8S,
				"failed to get rollout in k8s"),
			"failed to get rollout in k8s: deployment = %s, err = %v", node.Name, err)
	}

	un, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, nil, perror.Wrapf(
			herrors.NewErrGetFailed(herrors.ResourceInK8S,
				"failed to get rollout in k8s"),
			"failed to get rollout in k8s: deployment = %s, err = \"could not convert obj into unstructured\"", node.Name)
	}

	var instance *rolloutsv1alpha1.Rollout
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(un.UnstructuredContent(), &instance)
	if err != nil {
		return nil, un, err
	}
	return instance, un, nil
}

func (*rollout) getRolloutByNode(node *v1alpha1.ResourceNode,
	client *kube.Client) (*rolloutsv1alpha1.Rollout, *unstructured.Unstructured, error) {
	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  node.Version,
		Resource: "rollouts",
	}

	un, err := client.Dynamic.Resource(gvr).Namespace(node.Namespace).
		Get(context.TODO(), node.Name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, perror.Wrapf(
			herrors.NewErrGetFailed(herrors.ResourceInK8S,
				"failed to get rollout in k8s"),
			"failed to get rollout in k8s: deployment = %s, err = %v", node.Name, err)
	}

	var instance *rolloutsv1alpha1.Rollout
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(un.UnstructuredContent(), &instance)
	if err != nil {
		return nil, un, err
	}
	return instance, un, nil
}

func (r *rollout) IsHealthy(node *v1alpha1.ResourceNode,
	client *kube.Client) (bool, error) {
	instance, _, err := r.getRolloutByNode(node, client)
	if err != nil {
		return true, err
	}

	if instance == nil {
		return true, nil
	}

	labels := polymorphichelpers.MakeLabels(instance.Spec.Template.ObjectMeta.Labels)
	pods, err := client.Basic.CoreV1().Pods(instance.Namespace).
		List(context.TODO(), metav1.ListOptions{LabelSelector: labels, ResourceVersion: "0"})
	if err != nil {
		return true, err
	}
	log.Debugf(context.TODO(), "[workload rollout: %v]: list pods: count = %v", node.Name, len(pods.Items))

	count := 0
	required := 0
	if instance.Spec.Replicas != nil {
		required = int(*instance.Spec.Replicas)
	}
	log.Debugf(context.TODO(), "[workload rollout: %v]: required replicas = %v", node.Name, required)

	templateHashSum := computePodSpecHash(instance.Spec.Template.Spec)
OUTTER:
	for _, pod := range pods.Items {
		if pod.Status.Phase != "Running" {
			log.Debugf(context.TODO(), "[workload rollout: %v]: pod(%v) is not Running", node.Name, pod.Name)
			continue
		}
		hashSum := computePodSpecHash(pod.Spec)
		if templateHashSum != hashSum {
			log.Debugf(context.TODO(), "[workload rollout: %v]: pod(%v)'s hash is not matched", node.Name, pod.Name)
			continue
		}
		for k, v := range instance.Spec.Template.ObjectMeta.Annotations {
			if pod.Annotations[k] != v {
				log.Debugf(context.TODO(), "[workload rollout: %v]: pod(%v)'s annotation is not matched", node.Name, pod.Name)
				continue OUTTER
			}
		}
		count++
	}
	if count != required {
		log.Debugf(context.TODO(), "[workload rollout: %v]: required %v, has %v", node.Name, required, count)
		return false, nil
	}

	if instance.Status.CurrentStepIndex != nil {
		log.Debugf(context.TODO(),
			"[workload rollout: %v]: current step = %v, total steps = %v",
			node.Name, *instance.Status.CurrentStepIndex, instance.Spec.Strategy.Canary.Steps)
		return int(*instance.Status.CurrentStepIndex) == len(instance.Spec.Strategy.Canary.Steps), nil
	}
	return true, nil
}

func (r *rollout) ListPods(node *v1alpha1.ResourceNode,
	factory dynamicinformer.DynamicSharedInformerFactory) ([]corev1.Pod, error) {
	rolloutInformer := factory.ForResource(GVRRollout)
	podInformer := factory.ForResource(GVRPod)

	instance, _, err := r.getRollout(node, rolloutInformer)
	if err != nil {
		return nil, err
	}

	selector := labels.SelectorFromSet(instance.Spec.Template.ObjectMeta.Labels)
	if err != nil {
		return nil, herrors.NewErrGetFailed(herrors.ResourceInK8S,
			fmt.Sprintf("failed to get selectors for object %s/%s", instance.Namespace, instance.Name))
	}
	objs, err := podInformer.Lister().ByNamespace(node.Namespace).List(selector)
	if err != nil {
		return nil, err
	}

	pods := workload.ObjIntoPod(objs...)
	return pods, nil
}

func (r *rollout) GetSteps(node *v1alpha1.ResourceNode, client *kube.Client) (*workload.Step, error) {
	instance, un, err := r.getRolloutByNode(node, client)
	if err != nil {
		return nil, err
	}

	var replicasTotal = 1
	if instance.Spec.Replicas != nil {
		replicasTotal = int(*instance.Spec.Replicas)
	}

	if instance.Spec.Strategy.Canary == nil ||
		len(instance.Spec.Strategy.Canary.Steps) == 0 {
		return &workload.Step{
			Index:    0,
			Total:    1,
			Replicas: []int{replicasTotal},
		}, nil
	}

	replicasList := make([]int, 0)
	for _, step := range instance.Spec.Strategy.Canary.Steps {
		if step.SetWeight != nil {
			replicasList = append(replicasList, int(math.Ceil(float64(*step.SetWeight)/100*float64(replicasTotal))))
		} else if step.SetReplica != nil {
			replicasList = append(replicasList, int(*step.SetReplica))
		}
	}

	incrementReplicasList := make([]int, 0, len(replicasList))
	for i := 0; i < len(replicasList); i++ {
		replicas := replicasList[i]
		if i > 0 {
			replicas = replicasList[i] - replicasList[i-1]
		}
		incrementReplicasList = append(incrementReplicasList, replicas)
	}

	var stepIndex = 0
	// if steps changes, stepIndex = 0
	if instance.Status.CurrentStepHash == computeStepHash(instance) &&
		instance.Status.CurrentStepIndex != nil {
		index := float64(*instance.Status.CurrentStepIndex)
		index = math.Min(index, float64(len(instance.Spec.Strategy.Canary.Steps)))
		for i := 0; i < int(index); i++ {
			if instance.Spec.Strategy.Canary.Steps[i].SetWeight != nil ||
				instance.Spec.Strategy.Canary.Steps[i].SetReplica != nil {
				stepIndex++
			}
		}
	}

	autoPromote, _, err := unstructured.NestedBool(un.Object, "status", "autoPromote")
	if err != nil {
		return nil, err
	}

	bts, err := json.Marshal(map[string]interface{}{"currentIndex": *instance.Status.CurrentStepIndex})
	if err != nil {
		log.Errorf(context.TODO(), "marshal current step index failed: %v", err)
		bts = append(bts, []byte("{}")...)
	}

	extra := string(bts)

	// manual paused
	return &workload.Step{
		Index:        stepIndex,
		Total:        len(incrementReplicasList),
		Replicas:     incrementReplicasList,
		ManualPaused: instance.Spec.Paused,
		AutoPromote:  autoPromote,
		Extra:        &extra,
	}, nil
}

func (r *rollout) Action(actionName string, un *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var instance *rolloutsv1alpha1.Rollout
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(un.UnstructuredContent(), &instance)
	if err != nil {
		return un, perror.Wrapf(herrors.ErrParamInvalid, "convert to rollout failed: %v", err)
	}
	spec, ok := un.Object["spec"].(map[string]interface{})
	if !ok {
		return nil, perror.Wrapf(herrors.ErrParamInvalid, "spec not found")
	}
	status, ok := un.Object["status"].(map[string]interface{})
	if !ok {
		return nil, perror.Wrapf(herrors.ErrParamInvalid, "status not found")
	}
	switch actionName {
	case "resume":
		delete(status, "pauseConditions")
		spec["paused"] = false
	case "pause":
		spec["paused"] = true
	case "promote-full":
		steps := int32(len(instance.Spec.Strategy.Canary.Steps))
		spec["paused"] = false
		delete(status, "pauseConditions")
		status["currentStepIndex"] = steps
	case "promote":
		spec["paused"] = false
		delete(status, "pauseConditions")
	case "auto-promote":
		status["autoPromote"] = true
		delete(status, "pauseConditions")
		spec["paused"] = false
	case "cancel-auto-promote":
		delete(status, "autoPromote")
	default:
		return nil, perror.Wrapf(herrors.ErrParamInvalid, "unsupported action: %v", actionName)
	}
	return un, nil
}
