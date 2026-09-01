package actions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func newCronJob(suspended bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "CronJob",
		"metadata": map[string]any{
			"name":      "nightly",
			"namespace": "shop",
			"uid":       "cron-uid",
		},
		"spec": map[string]any{
			"schedule": "0 2 * * *",
			"suspend":  suspended,
			"jobTemplate": map[string]any{
				"metadata": map[string]any{
					"labels":      map[string]any{"app": "backup"},
					"annotations": map[string]any{"owner": "platform"},
				},
				"spec": map[string]any{
					"backoffLimit": int64(2),
					"template": map[string]any{
						"spec": map[string]any{"restartPolicy": "Never"},
					},
				},
			},
		},
	}}
}

func cronJobRef() api.ObjectRef {
	return api.ObjectRef{
		Group:     "batch",
		Version:   "v1",
		Resource:  "cronjobs",
		Namespace: "shop",
		Name:      "nightly",
	}
}

func readCronJob(t *testing.T, client *dynamicfake.FakeDynamicClient) *unstructured.Unstructured {
	t.Helper()
	got, err := client.Resource(cronJobGVR).
		Namespace("shop").
		Get(context.Background(), "nightly", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return got
}

func TestSuspendSetsTheFlag(t *testing.T) {
	client := dynClient(newCronJob(false))
	seen := recordPatches(client)
	service := serviceFor(client, k8sfake.NewClientset())

	result, err := service.Do(context.Background(), Request{Ref: cronJobRef(), Action: Suspend}, stamp)
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}

	if len(*seen) != 1 {
		t.Fatalf("sent %d patches, want 1", len(*seen))
	}
	if (*seen)[0].body != `{"spec":{"suspend":true}}` {
		t.Fatalf("patch = %s", (*seen)[0].body)
	}
	if (*seen)[0].subresource != "" {
		t.Fatalf("subresource = %q, want the object itself", (*seen)[0].subresource)
	}
	if result.Action != string(Suspend) {
		t.Fatalf("action = %q, want suspend", result.Action)
	}
	if !strings.Contains(result.Message, "nightly") {
		t.Fatalf("message = %q, want it to name the cron job", result.Message)
	}
	if suspend, _, _ := unstructured.NestedBool(readCronJob(t, client).Object, "spec", "suspend"); !suspend {
		t.Fatal("the cron job is still running its schedule")
	}
}

func TestResumeClearsTheFlag(t *testing.T) {
	client := dynClient(newCronJob(true))
	seen := recordPatches(client)
	service := serviceFor(client, k8sfake.NewClientset())

	result, err := service.Do(context.Background(), Request{Ref: cronJobRef(), Action: Resume}, stamp)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}

	if (*seen)[0].body != `{"spec":{"suspend":false}}` {
		t.Fatalf("patch = %s", (*seen)[0].body)
	}
	if result.Action != string(Resume) {
		t.Fatalf("action = %q, want resume", result.Action)
	}
	if suspend, _, _ := unstructured.NestedBool(readCronJob(t, client).Object, "spec", "suspend"); suspend {
		t.Fatal("the cron job is still suspended")
	}
}

func TestSuspendReportsARefusal(t *testing.T) {
	client := dynClient()
	service := serviceFor(client, k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: cronJobRef(), Action: Suspend}, stamp)

	if err == nil {
		t.Fatal("a cron job that is not there was suspended anyway")
	}
}

func triggeredJob(t *testing.T, client *dynamicfake.FakeDynamicClient) *unstructured.Unstructured {
	t.Helper()
	list, err := client.Resource(jobGVR).Namespace("shop").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("made %d jobs, want 1", len(list.Items))
	}
	return &list.Items[0]
}

func TestTriggerMakesAJobFromTheTemplate(t *testing.T) {
	client := dynClient(newCronJob(false))
	service := serviceFor(client, k8sfake.NewClientset())

	result, err := service.Do(context.Background(), Request{Ref: cronJobRef(), Action: Trigger}, stamp)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}

	job := triggeredJob(t, client)
	if job.GetAPIVersion() != "batch/v1" || job.GetKind() != "Job" {
		t.Fatalf("made a %s %s", job.GetAPIVersion(), job.GetKind())
	}
	limit, _, _ := unstructured.NestedInt64(job.Object, "spec", "backoffLimit")
	if limit != 2 {
		t.Fatalf("backoffLimit = %d, want the template's", limit)
	}
	policy, _, _ := unstructured.NestedString(job.Object, "spec", "template", "spec", "restartPolicy")
	if policy != "Never" {
		t.Fatalf("restartPolicy = %q, want the template's", policy)
	}
	if !strings.Contains(result.Message, job.GetName()) {
		t.Fatalf("message = %q, want it to name the job it started", result.Message)
	}
}

func TestATriggeredJobIsNamedAfterTheCronJobAndTheMoment(t *testing.T) {
	client := dynClient(newCronJob(false))
	service := serviceFor(client, k8sfake.NewClientset())

	if _, err := service.Do(context.Background(), Request{Ref: cronJobRef(), Action: Trigger}, stamp); err != nil {
		t.Fatalf("trigger: %v", err)
	}

	want := fmt.Sprintf("nightly-%d", stamp.Unix())
	if got := triggeredJob(t, client).GetName(); got != want {
		t.Fatalf("job name = %q, want %q", got, want)
	}
}

func TestATriggeredJobCarriesTheTemplatesLabelsAndAnnotations(t *testing.T) {
	client := dynClient(newCronJob(false))
	service := serviceFor(client, k8sfake.NewClientset())

	if _, err := service.Do(context.Background(), Request{Ref: cronJobRef(), Action: Trigger}, stamp); err != nil {
		t.Fatalf("trigger: %v", err)
	}

	job := triggeredJob(t, client)
	if job.GetLabels()["app"] != "backup" {
		t.Fatalf("labels = %v, want the template's", job.GetLabels())
	}
	if job.GetAnnotations()["owner"] != "platform" {
		t.Fatalf("annotations = %v, want the template's", job.GetAnnotations())
	}
	if job.GetAnnotations()[instantiateAnnotation] != "manual" {
		t.Fatalf("annotations = %v, want the run marked as started by hand", job.GetAnnotations())
	}
}

func TestATriggeredJobIsOwnedByTheCronJobButNotControlledByIt(t *testing.T) {
	client := dynClient(newCronJob(false))
	service := serviceFor(client, k8sfake.NewClientset())

	if _, err := service.Do(context.Background(), Request{Ref: cronJobRef(), Action: Trigger}, stamp); err != nil {
		t.Fatalf("trigger: %v", err)
	}

	owners := triggeredJob(t, client).GetOwnerReferences()
	if len(owners) != 1 {
		t.Fatalf("owners = %v, want the cron job alone", owners)
	}
	if owners[0].Kind != "CronJob" || owners[0].Name != "nightly" || string(owners[0].UID) != "cron-uid" {
		t.Fatalf("owner = %+v, want the cron job", owners[0])
	}
	if owners[0].Controller != nil && *owners[0].Controller {
		t.Fatal("the job was marked as controlled by the schedule")
	}
}

func TestATriggeredJobKeepsALabellessTemplateLabelless(t *testing.T) {
	cron := newCronJob(false)
	unstructured.RemoveNestedField(cron.Object, "spec", "jobTemplate", "metadata")
	client := dynClient(cron)
	service := serviceFor(client, k8sfake.NewClientset())

	if _, err := service.Do(context.Background(), Request{Ref: cronJobRef(), Action: Trigger}, stamp); err != nil {
		t.Fatalf("trigger: %v", err)
	}

	job := triggeredJob(t, client)
	if len(job.GetLabels()) != 0 {
		t.Fatalf("labels = %v, want none", job.GetLabels())
	}
	if job.GetAnnotations()[instantiateAnnotation] != "manual" {
		t.Fatalf("annotations = %v, want the run still marked", job.GetAnnotations())
	}
}

func TestACronJobWithNoTemplateCannotBeTriggered(t *testing.T) {
	cron := newCronJob(false)
	unstructured.RemoveNestedField(cron.Object, "spec", "jobTemplate")
	client := dynClient(cron)
	service := serviceFor(client, k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: cronJobRef(), Action: Trigger}, stamp)

	if err == nil {
		t.Fatal("a cron job with no template was triggered anyway")
	}
	list, _ := client.Resource(jobGVR).Namespace("shop").List(context.Background(), metav1.ListOptions{})
	if len(list.Items) != 0 {
		t.Fatalf("made %d jobs anyway", len(list.Items))
	}
}

func TestTriggeringACronJobThatIsNotThereIsReported(t *testing.T) {
	service := serviceFor(dynClient(), k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: cronJobRef(), Action: Trigger}, stamp)

	if err == nil {
		t.Fatal("a cron job that is not there was triggered anyway")
	}
}

func TestTriggerReportsWhenTheJobCannotBeCreated(t *testing.T) {
	client := dynClient(newCronJob(false))
	client.PrependReactor("create", "jobs", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("jobs are forbidden")
	})
	service := serviceFor(client, k8sfake.NewClientset())

	_, err := service.Do(context.Background(), Request{Ref: cronJobRef(), Action: Trigger}, stamp)

	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("error = %v, want the create refusal", err)
	}
	list, listErr := client.Resource(jobGVR).Namespace("shop").List(context.Background(), metav1.ListOptions{})
	if listErr != nil {
		t.Fatalf("list jobs: %v", listErr)
	}
	if len(list.Items) != 0 {
		t.Fatalf("made %d jobs after the create was refused", len(list.Items))
	}
}
