package helper

import (
	"context"
	"testing"
	"time"

	fakeworkclient "github.com/open-cluster-management/api/client/work/clientset/versioned/fake"
	workapiv1 "github.com/open-cluster-management/api/work/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"
)

func newCondition(name, status, reason, message string, lastTransition *metav1.Time) workapiv1.StatusCondition {
	ret := workapiv1.StatusCondition{
		Type:    name,
		Status:  metav1.ConditionStatus(status),
		Reason:  reason,
		Message: message,
	}
	if lastTransition != nil {
		ret.LastTransitionTime = *lastTransition
	}
	return ret
}

func updateSpokeClusterConditionFn(cond workapiv1.StatusCondition) UpdateManifestWorkStatusFunc {
	return func(oldStatus *workapiv1.ManifestWorkStatus) error {
		SetStatusCondition(&oldStatus.Conditions, cond)
		return nil
	}
}

func newManifestCondition(ordinal int32, resource string, conds ...workapiv1.StatusCondition) workapiv1.ManifestCondition {
	return workapiv1.ManifestCondition{
		ResourceMeta: workapiv1.ManifestResourceMeta{Ordinal: ordinal, Resource: resource},
		Conditions:   conds,
	}
}

// TestUpdateStatusCondition tests UpdateManifestWorkStatus function
func TestUpdateStatusCondition(t *testing.T) {
	nowish := metav1.Now()
	beforeish := metav1.Time{Time: nowish.Add(-10 * time.Second)}
	afterish := metav1.Time{Time: nowish.Add(10 * time.Second)}

	cases := []struct {
		name               string
		startingConditions []workapiv1.StatusCondition
		newCondition       workapiv1.StatusCondition
		expectedUpdated    bool
		expectedConditions []workapiv1.StatusCondition
	}{
		{
			name:               "add to empty",
			startingConditions: []workapiv1.StatusCondition{},
			newCondition:       newCondition("test", "True", "my-reason", "my-message", nil),
			expectedUpdated:    true,
			expectedConditions: []workapiv1.StatusCondition{newCondition("test", "True", "my-reason", "my-message", nil)},
		},
		{
			name: "add to non-conflicting",
			startingConditions: []workapiv1.StatusCondition{
				newCondition("two", "True", "my-reason", "my-message", nil),
			},
			newCondition:    newCondition("one", "True", "my-reason", "my-message", nil),
			expectedUpdated: true,
			expectedConditions: []workapiv1.StatusCondition{
				newCondition("two", "True", "my-reason", "my-message", nil),
				newCondition("one", "True", "my-reason", "my-message", nil),
			},
		},
		{
			name: "change existing status",
			startingConditions: []workapiv1.StatusCondition{
				newCondition("two", "True", "my-reason", "my-message", nil),
				newCondition("one", "True", "my-reason", "my-message", nil),
			},
			newCondition:    newCondition("one", "False", "my-different-reason", "my-othermessage", nil),
			expectedUpdated: true,
			expectedConditions: []workapiv1.StatusCondition{
				newCondition("two", "True", "my-reason", "my-message", nil),
				newCondition("one", "False", "my-different-reason", "my-othermessage", nil),
			},
		},
		{
			name: "leave existing transition time",
			startingConditions: []workapiv1.StatusCondition{
				newCondition("two", "True", "my-reason", "my-message", nil),
				newCondition("one", "True", "my-reason", "my-message", &beforeish),
			},
			newCondition:    newCondition("one", "True", "my-reason", "my-message", &afterish),
			expectedUpdated: false,
			expectedConditions: []workapiv1.StatusCondition{
				newCondition("two", "True", "my-reason", "my-message", nil),
				newCondition("one", "True", "my-reason", "my-message", &beforeish),
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeWorkClient := fakeworkclient.NewSimpleClientset(&workapiv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{Name: "work1", Namespace: "cluster1"},
				Status: workapiv1.ManifestWorkStatus{
					Conditions: c.startingConditions,
				},
			})

			status, updated, err := UpdateManifestWorkStatus(
				context.TODO(),
				fakeWorkClient.WorkV1().ManifestWorks("cluster1"),
				"work1",
				updateSpokeClusterConditionFn(c.newCondition),
			)
			if err != nil {
				t.Errorf("unexpected err: %v", err)
			}
			if updated != c.expectedUpdated {
				t.Errorf("expected %t, but %t", c.expectedUpdated, updated)
			}
			for i := range c.expectedConditions {
				expected := c.expectedConditions[i]
				actual := status.Conditions[i]
				if expected.LastTransitionTime == (metav1.Time{}) {
					actual.LastTransitionTime = metav1.Time{}
				}
				if !equality.Semantic.DeepEqual(expected, actual) {
					t.Errorf(diff.ObjectDiff(expected, actual))
				}
			}
		})
	}
}

// TestSetManifestCondition tests SetManifestCondition function
func TestMergeManifestConditions(t *testing.T) {
	transitionTime := metav1.Now()

	cases := []struct {
		name               string
		startingConditions []workapiv1.ManifestCondition
		newConditions      []workapiv1.ManifestCondition
		expectedConditions []workapiv1.ManifestCondition
	}{
		{
			name:               "add to empty",
			startingConditions: []workapiv1.ManifestCondition{},
			newConditions: []workapiv1.ManifestCondition{
				newManifestCondition(0, "resource1", newCondition("one", "True", "my-reason", "my-message", nil)),
			},
			expectedConditions: []workapiv1.ManifestCondition{
				newManifestCondition(0, "resource1", newCondition("one", "True", "my-reason", "my-message", nil)),
			},
		},
		{
			name: "add new conddtion",
			startingConditions: []workapiv1.ManifestCondition{
				newManifestCondition(0, "resource1", newCondition("one", "True", "my-reason", "my-message", nil)),
			},
			newConditions: []workapiv1.ManifestCondition{
				newManifestCondition(0, "resource1", newCondition("one", "True", "my-reason", "my-message", nil)),
				newManifestCondition(0, "resource2", newCondition("two", "True", "my-reason", "my-message", nil)),
			},
			expectedConditions: []workapiv1.ManifestCondition{
				newManifestCondition(0, "resource1", newCondition("one", "True", "my-reason", "my-message", nil)),
				newManifestCondition(0, "resource2", newCondition("two", "True", "my-reason", "my-message", nil)),
			},
		},
		{
			name: "update existing",
			startingConditions: []workapiv1.ManifestCondition{
				newManifestCondition(0, "resource1", newCondition("one", "True", "my-reason", "my-message", nil)),
			},
			newConditions: []workapiv1.ManifestCondition{
				newManifestCondition(0, "resource1", newCondition("two", "True", "my-reason", "my-message", nil)),
			},
			expectedConditions: []workapiv1.ManifestCondition{
				newManifestCondition(0, "resource1", newCondition("two", "True", "my-reason", "my-message", nil)),
			},
		},
		{
			name: "remove useless",
			startingConditions: []workapiv1.ManifestCondition{
				newManifestCondition(0, "resource1", newCondition("one", "True", "my-reason", "my-message", nil)),
				newManifestCondition(1, "resource2", newCondition("two", "True", "my-reason", "my-message", &transitionTime)),
			},
			newConditions: []workapiv1.ManifestCondition{
				newManifestCondition(0, "resource2", newCondition("two", "True", "my-reason", "my-message", nil)),
			},
			expectedConditions: []workapiv1.ManifestCondition{
				newManifestCondition(0, "resource2", newCondition("two", "True", "my-reason", "my-message", &transitionTime)),
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			merged := MergeManifestConditions(c.startingConditions, c.newConditions)

			if len(merged) != len(c.expectedConditions) {
				t.Errorf("expected condition size %d but got: %d", len(c.expectedConditions), len(merged))
			}

			for i, expectedCondition := range c.expectedConditions {
				actualCondition := merged[i]
				if len(actualCondition.Conditions) != len(expectedCondition.Conditions) {
					t.Errorf("expected condition size %d but got: %d", len(expectedCondition.Conditions), len(actualCondition.Conditions))
				}
				for j, expect := range expectedCondition.Conditions {
					if expect.LastTransitionTime == (metav1.Time{}) {
						actualCondition.Conditions[j].LastTransitionTime = metav1.Time{}
					}
				}

				if !equality.Semantic.DeepEqual(actualCondition, expectedCondition) {
					t.Errorf(diff.ObjectDiff(actualCondition, expectedCondition))
				}
			}
		})
	}
}

func TestMergeStatusConditions(t *testing.T) {
	transitionTime := metav1.Now()

	cases := []struct {
		name               string
		startingConditions []workapiv1.StatusCondition
		newConditions      []workapiv1.StatusCondition
		expectedConditions []workapiv1.StatusCondition
	}{
		{
			name: "add status condition",
			newConditions: []workapiv1.StatusCondition{
				newCondition("one", "True", "my-reason", "my-message", nil),
			},
			expectedConditions: []workapiv1.StatusCondition{
				newCondition("one", "True", "my-reason", "my-message", nil),
			},
		},
		{
			name: "merge status condition",
			startingConditions: []workapiv1.StatusCondition{
				newCondition("one", "True", "my-reason", "my-message", nil),
			},
			newConditions: []workapiv1.StatusCondition{
				newCondition("one", "False", "my-reason", "my-message", nil),
				newCondition("two", "True", "my-reason", "my-message", nil),
			},
			expectedConditions: []workapiv1.StatusCondition{
				newCondition("one", "False", "my-reason", "my-message", nil),
				newCondition("two", "True", "my-reason", "my-message", nil),
			},
		},
		{
			name: "remove old status condition",
			startingConditions: []workapiv1.StatusCondition{
				newCondition("one", "False", "my-reason", "my-message", &transitionTime),
				newCondition("two", "True", "my-reason", "my-message", nil),
			},
			newConditions: []workapiv1.StatusCondition{
				newCondition("one", "False", "my-reason", "my-message", nil),
			},
			expectedConditions: []workapiv1.StatusCondition{
				newCondition("one", "False", "my-reason", "my-message", &transitionTime),
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			merged := MergeStatusConditions(c.startingConditions, c.newConditions)
			for i, expect := range c.expectedConditions {
				actual := merged[i]
				if expect.LastTransitionTime == (metav1.Time{}) {
					actual.LastTransitionTime = metav1.Time{}
				}
				if !equality.Semantic.DeepEqual(actual, expect) {
					t.Errorf(diff.ObjectDiff(actual, expect))
				}
			}
		})
	}
}
