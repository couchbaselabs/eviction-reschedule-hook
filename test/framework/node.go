package framework

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (tx *TestCluster) MustAddNodeCondition(t *testing.T, nodeName, conditionType string) {
	condition := corev1.NodeCondition{
		Type:   corev1.NodeConditionType(conditionType),
		Status: corev1.ConditionTrue,
	}

	retryFetch(t, nodeName, func() (interface{}, error) {
		node, err := tx.client.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		node.Status.Conditions = append(node.Status.Conditions, condition)
		_, err = tx.client.CoreV1().Nodes().UpdateStatus(context.TODO(), node, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		return nil, nil
	})
}

func (tx *TestCluster) MustAddNodeTaint(t *testing.T, nodeName, taintKey string) {
	taint := corev1.Taint{
		Key:    taintKey,
		Effect: corev1.TaintEffectPreferNoSchedule,
	}

	retryFetch(t, nodeName, func() (interface{}, error) {
		node, err := tx.client.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		for _, existingTaint := range node.Spec.Taints {
			if existingTaint.Key == taintKey {
				// Taint already exists, no need to add it again
				return nil, nil
			}
		}

		node.Spec.Taints = append(node.Spec.Taints, taint)
		_, err = tx.client.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		return nil, nil
	})
}

func (tc *TestCluster) MustRemoveTaintFromAllNodes(t *testing.T, taintKey string) {
	retryFetch(t, taintKey, func() (interface{}, error) {
		nodes, err := tc.client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, err
		}

		for _, node := range nodes.Items {
			var newTaints []corev1.Taint
			for _, taint := range node.Spec.Taints {
				if taint.Key != taintKey {
					newTaints = append(newTaints, taint)
				}
			}
			node.Spec.Taints = newTaints
			if _, err := tc.client.CoreV1().Nodes().Update(context.TODO(), &node, metav1.UpdateOptions{}); err != nil {
				return nil, err
			}
		}

		return nil, nil
	})
}

func (tc *TestCluster) MustRemoveConditionFromAllNodes(t *testing.T, conditionType string) {
	retryFetch(t, conditionType, func() (interface{}, error) {
		nodes, err := tc.client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, err
		}

		for _, node := range nodes.Items {
			for i, condition := range node.Status.Conditions {
				if string(condition.Type) == conditionType {
					node.Status.Conditions = append(node.Status.Conditions[:i], node.Status.Conditions[i+1:]...)
					break
				}
			}
			if _, err := tc.client.CoreV1().Nodes().UpdateStatus(context.TODO(), &node, metav1.UpdateOptions{}); err != nil {
				return nil, err
			}
		}

		return nil, nil
	})
}
