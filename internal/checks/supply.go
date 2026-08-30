package checks

import (
	"slices"
	"strconv"
	"strings"
)

const (
	digestMarker      = "@sha256:"
	dockerHub         = "docker.io"
	dockerHubIndex    = "index.docker.io"
	alwaysPull        = "Always"
	latestTag         = "latest"
	nameLabel         = "app.kubernetes.io/name"
	defaultNamespace  = "default"
	groupOtherBits    = 0o077
	credentialMinimum = 4
)

var credentialWords = []string{
	"PASSWORD",
	"PASSWD",
	"SECRET",
	"TOKEN",
	"APIKEY",
	"API_KEY",
	"PRIVATE_KEY",
	"ACCESS_KEY",
	"CREDENTIAL",
}

var publicRegistries = []string{
	dockerHub,
	dockerHubIndex,
	"gcr.io",
	"ghcr.io",
	"public.ecr.aws",
	"quay.io",
	"registry.k8s.io",
	"mcr.microsoft.com",
}

func supplyChecks() []check {
	return []check{
		{
			id:       "image-not-digest-pinned",
			title:    "Image not pinned to a digest",
			category: categoryReliability,
			severity: severityLow,
			wrong:    "A tag can be moved, so two pods of the same workload can be running different code and a rollback has no fixed target.",
			remedy:   "Append @sha256:… to the image, keeping the tag for readability.",
			find:     overContainers(imageNotPinned),
		},
		{
			id:       "image-from-docker-hub",
			title:    "Image pulled from Docker Hub",
			category: categoryReliability,
			severity: severityLow,
			wrong:    "Anonymous pulls are rate limited per node address, so a rollout can fail on pull rather than on anything you changed.",
			remedy:   "Mirror the image into a registry you control and pull it from there.",
			find:     overContainers(imageFromDockerHub),
		},
		{
			id:       "pull-policy-not-always",
			arguable: true,
			title:    "Mutable tag not pulled every time",
			category: categoryReliability,
			severity: severityLow,
			wrong:    "The node keeps whatever it cached, so a moved tag reaches new nodes and not old ones. A judgement call: pinning the digest instead makes this moot.",
			remedy:   "Set imagePullPolicy to Always, or pin the image to a digest.",
			find:     overContainers(pullPolicyNotAlways),
		},
		{
			id:       "private-registry-no-pull-secret",
			arguable: true,
			title:    "Private registry with no pull secret",
			category: categoryReliability,
			severity: severityLow,
			wrong:    "If the registry ever needs credentials the pull fails on a node that has none cached. A judgement call: a registry open to the cluster's network needs nothing.",
			remedy:   "Add imagePullSecrets, or attach the secret to the pod's ServiceAccount.",
			find:     overSubjects(privateRegistryNoSecret),
		},
		{
			id:         "secret-in-env-literal",
			title:      "Credential written into the manifest",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{nsaCisa},
			wrong:      "The value is in the object, so it is in every backup, every audit log and anything that can read the workload.",
			remedy:     "Move it to a Secret and reference it with valueFrom.secretKeyRef.",
			find:       overContainers(secretInEnvLiteral),
		},
		{
			id:         "env-from-secret-wholesale",
			title:      "Whole Secret loaded into the environment",
			category:   categorySecurity,
			severity:   severityMedium,
			frameworks: []string{nsaCisa},
			wrong:      "Every key lands in the environment, including ones added later, and the environment is readable by anything that can exec into the container.",
			remedy:     "Name the keys you need with valueFrom.secretKeyRef, or mount the Secret as a file.",
			find:       overContainers(envFromSecret),
		},
		{
			id:         "secret-volume-world-readable",
			arguable:   true,
			title:      "Secret file readable beyond its owner",
			category:   categorySecurity,
			severity:   severityLow,
			frameworks: []string{nsaCisa},
			wrong:      "Any process in the container, whatever user it runs as, can read the secret off disk. A judgement call: 0644 is what the apiserver fills in when nobody chooses, so this is true of almost every secret volume until somebody tightens it.",
			remedy:     "Set the volume's defaultMode to 0400.",
			find:       overSubjects(secretVolumeReadable),
		},
		{
			id:       "default-namespace",
			title:    "Running in the default namespace",
			category: categoryReliability,
			severity: severityLow,
			wrong:    "Nothing separates it from anything else somebody applies without thinking, and namespace-scoped policy and quota cannot be aimed at it.",
			remedy:   "Move it to a namespace named after what it is.",
			find:     overSubjects(inDefaultNamespace),
		},
		{
			id:       "missing-recommended-labels",
			arguable: true,
			title:    "No app.kubernetes.io/name",
			category: categoryReliability,
			severity: severityLow,
			wrong:    "Nothing ties the object to the application it belongs to, so selectors, dashboards and cost reports have to guess. A judgement call: your own convention may be a different label entirely.",
			remedy:   "Set app.kubernetes.io/name, and instance and version alongside it.",
			find:     overSubjects(missingNameLabel),
		},
		{
			id:       "selector-template-mismatch",
			title:    "Selector does not match its own template",
			category: categoryReliability,
			severity: severityHigh,
			wrong:    "The controller creates pods it then cannot find, so it creates them again, for ever.",
			remedy:   "Make every label in spec.selector.matchLabels appear on the pod template.",
			find:     overSubjects(selectorMismatch),
		},
		{
			id:       "cpu-limit-set",
			arguable: true,
			title:    "CPU limit set",
			category: categoryEfficiency,
			severity: severityLow,
			wrong:    "A CPU limit throttles the container even when the node is idle, which shows up as latency nobody can account for. A judgement call: a limit is the right answer where one tenant must not crowd out another.",
			remedy:   "Drop resources.limits.cpu and let the request do the scheduling.",
			find:     overContainers(cpuLimitSet),
		},
	}
}

func imageOf(container Container) string {
	return stringAt(container.Spec, "image")
}

func imageNotPinned(_ Subject, container Container) (string, string) {
	image := imageOf(container)
	if image == "" || strings.Contains(image, digestMarker) {
		return "", ""
	}
	return image + " names no digest", ""
}

func registryOf(image string) string {
	head, _, found := strings.Cut(image, "/")
	if !found {
		return dockerHub
	}
	if !strings.ContainsAny(head, ".:") && head != "localhost" {
		return dockerHub
	}
	return head
}

func imageFromDockerHub(_ Subject, container Container) (string, string) {
	image := imageOf(container)
	if image == "" {
		return "", ""
	}
	registry := registryOf(image)
	if registry != dockerHub && registry != dockerHubIndex {
		return "", ""
	}
	return image + " comes from Docker Hub", ""
}

func pullPolicyNotAlways(_ Subject, container Container) (string, string) {
	image := imageOf(container)
	if image == "" || strings.Contains(image, digestMarker) {
		return "", ""
	}
	tag := tagOf(image)
	if tag == "" || tag == latestTag {
		return "", ""
	}
	policy := stringAt(container.Spec, "imagePullPolicy")
	if policy == alwaysPull {
		return "", ""
	}
	named := policy
	if named == "" {
		named = "unset, which caches"
	}
	return "imagePullPolicy is " + named + " on the mutable tag " + tag, ""
}

func privateRegistryNoSecret(subject Subject) (string, string) {
	if listedItems(subject.Pod, "imagePullSecrets") > 0 {
		return "", ""
	}
	registry := firstPrivateRegistry(subject)
	if registry == "" {
		return "", ""
	}
	return "pulls from " + registry + " with no imagePullSecrets", ""
}

func firstPrivateRegistry(subject Subject) string {
	for _, entry := range subject.Containers {
		image := imageOf(entry)
		if image == "" {
			continue
		}
		registry := registryOf(image)
		if slices.Contains(publicRegistries, registry) {
			continue
		}
		return registry
	}
	return ""
}

func looksLikeCredential(name string) bool {
	upper := strings.ToUpper(name)
	for _, word := range credentialWords {
		if strings.Contains(upper, word) {
			return true
		}
	}
	return false
}

func secretInEnvLiteral(_ Subject, container Container) (string, string) {
	for _, entry := range envEntries(container) {
		name := stringAt(entry, "name")
		if !looksLikeCredential(name) {
			continue
		}
		value := stringAt(entry, "value")
		if len(value) < credentialMinimum {
			continue
		}
		return name + " is set to a literal value in the manifest", ""
	}
	return "", ""
}

func envEntries(container Container) []map[string]any {
	listed, ok := container.Spec["env"].([]any)
	if !ok {
		return nil
	}
	out := []map[string]any{}
	for _, entry := range listed {
		item, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		out = append(out, item)
	}
	return out
}

func envFromSecret(_ Subject, container Container) (string, string) {
	listed, ok := container.Spec["envFrom"].([]any)
	if !ok {
		return "", ""
	}
	for _, entry := range listed {
		item, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		ref, hasRef := item["secretRef"].(map[string]any)
		if !hasRef {
			continue
		}
		return "envFrom loads every key of the secret " + stringAt(ref, "name"), ""
	}
	return "", ""
}

func secretVolumeReadable(subject Subject) (string, string) {
	volumes, ok := subject.Pod["volumes"].([]any)
	if !ok {
		return "", ""
	}
	for _, entry := range volumes {
		volume, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		secret, hasSecret := volume["secret"].(map[string]any)
		if !hasSecret {
			continue
		}
		mode, set := numberAt(secret, "defaultMode")
		if !set || mode&groupOtherBits == 0 {
			continue
		}
		return "volume " + stringAt(volume, "name") + " mounts a secret with defaultMode " +
			"0" + strconv.FormatInt(mode, 8), ""
	}
	return "", ""
}

func inDefaultNamespace(subject Subject) (string, string) {
	if subject.Ref.Namespace != defaultNamespace {
		return "", ""
	}
	return "runs in the default namespace", ""
}

func missingNameLabel(subject Subject) (string, string) {
	if subject.Object.GetLabels()[nameLabel] != "" {
		return "", ""
	}
	return "no " + nameLabel + " label", ""
}

func templateLabels(subject Subject) map[string]any {
	path := templatePath(subject.Kind)
	if len(path) < 2 {
		return nil
	}
	meta := specAt(subject.Object, append(path[:len(path)-1], "metadata")...)
	labels, ok := meta["labels"].(map[string]any)
	if !ok {
		return nil
	}
	return labels
}

func selectorMismatch(subject Subject) (string, string) {
	if !isSpreadable(subject.Kind) && subject.Kind != daemonSetKind {
		return "", ""
	}
	wanted := selectorLabels(subject)
	if len(wanted) == 0 {
		return "", ""
	}
	carried := templateLabels(subject)
	missing := []string{}
	for key, value := range wanted {
		if stringAt(carried, key) != value {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return "", ""
	}
	slices.Sort(missing)
	return "the pod template does not carry " + strings.Join(missing, ", "), ""
}

func selectorLabels(subject Subject) map[string]string {
	selector, ok := specAt(subject.Object, specField)["selector"].(map[string]any)
	if !ok {
		return nil
	}
	pairs, ok := selector["matchLabels"].(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for key, value := range pairs {
		text, isString := value.(string)
		if !isString {
			continue
		}
		out[key] = text
	}
	return out
}

func cpuLimitSet(_ Subject, container Container) (string, string) {
	limit, ok := quantityAt(container.Spec, limits, cpuName)
	if !ok {
		return "", ""
	}
	return "resources.limits.cpu is " + limit.String(), ""
}
