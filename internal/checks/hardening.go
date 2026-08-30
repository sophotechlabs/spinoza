package checks

import (
	"slices"
	"strconv"
	"strings"
)

const (
	unconfined       = "Unconfined"
	runtimeDefault   = "RuntimeDefault"
	seccompField     = "seccompProfile"
	apparmorField    = "appArmorProfile"
	apparmorPrefix   = "container.apparmor.security.beta.kubernetes.io/"
	dropAll          = "ALL"
	netRaw           = "NET_RAW"
	sensitiveMount   = "hostPath"
	windowsOptions   = "windowsOptions"
	seLinuxField     = "seLinuxOptions"
	procMountField   = "procMount"
	procMountUnmask  = "Unmasked"
	sysctlsField     = "sysctls"
	hostProcessField = "hostProcess"
)

var safeSysctls = []string{
	"kernel.shm_rmid_forced",
	"net.ipv4.ip_local_port_range",
	"net.ipv4.ip_local_reserved_ports",
	"net.ipv4.ip_unprivileged_port_start",
	"net.ipv4.ping_group_range",
	"net.ipv4.tcp_syncookies",
}

var sensitiveHostPaths = []string{
	"/",
	"/boot",
	"/dev",
	"/etc",
	"/home",
	"/proc",
	"/root",
	"/run",
	"/sys",
	"/var/lib/kubelet",
	"/var/log",
	"/var/run",
}

var restrictedVolumeTypes = []string{
	"configMap",
	"csi",
	"downwardAPI",
	"emptyDir",
	"ephemeral",
	"persistentVolumeClaim",
	"projected",
	"secret",
}

var seLinuxNarrowing = []string{"user", "role", "type"}

func hardeningChecks() []check {
	return []check{
		{
			id:         "seccomp-unset",
			title:      "No seccomp profile",
			category:   categorySecurity,
			severity:   severityMedium,
			frameworks: []string{pssRestricted, nsaCisa},
			wrong:      "Every syscall the kernel offers is reachable, including the ones no container needs.",
			remedy:     "Set securityContext.seccompProfile.type to RuntimeDefault on the pod or the container.",
			find:       overContainers(seccompUnset),
		},
		{
			id:         "seccomp-unconfined",
			title:      "Seccomp explicitly disabled",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{pssBaseline, nsaCisa},
			wrong:      "Unconfined turns the syscall filter off, which is weaker than leaving it unset on most runtimes.",
			remedy:     "Change securityContext.seccompProfile.type to RuntimeDefault.",
			find:       overContainers(seccompUnconfined),
		},
		{
			id:         "apparmor-unconfined",
			title:      "AppArmor disabled",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{pssBaseline},
			wrong:      "The container is exempt from the node's AppArmor policy.",
			remedy:     "Drop the unconfined profile and let the runtime default apply.",
			find:       overContainers(apparmorUnconfined),
		},
		{
			id:         "selinux-options-set",
			title:      "SELinux type or role overridden",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{pssBaseline},
			wrong:      "Choosing your own SELinux user, role or type can hand the container a domain wider than the default.",
			remedy:     "Remove seLinuxOptions.user, .role and .type; level is the only field that is safe to set.",
			find:       overContainers(seLinuxWidened),
		},
		{
			id:         "proc-mount-unmasked",
			title:      "/proc mounted unmasked",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{pssBaseline},
			wrong:      "The masked and read-only paths under /proc are exposed, which is a documented break-out surface.",
			remedy:     "Remove securityContext.procMount, which defaults to Default.",
			find:       overContainers(procMountUnmasked),
		},
		{
			id:         "unsafe-sysctls",
			title:      "Unsafe sysctls set",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{pssBaseline},
			wrong:      "An unsafe sysctl is namespaced badly or not at all, so it can reach the node and the pods beside it.",
			remedy:     "Remove the sysctl, or move the setting into the node's own configuration.",
			find:       overSubjects(unsafeSysctls),
		},
		{
			id:         "host-ports",
			title:      "Host port bound",
			category:   categorySecurity,
			severity:   severityMedium,
			frameworks: []string{pssBaseline},
			wrong:      "The port is claimed on the node itself, so the pod is reachable around any Service and collides with anything else wanting it.",
			remedy:     "Drop hostPort and publish the container through a Service.",
			find:       overContainers(hostPorts),
		},
		{
			id:         "host-path-volume",
			title:      "Node filesystem mounted",
			category:   categorySecurity,
			severity:   severityMedium,
			frameworks: []string{pssBaseline, nsaCisa},
			wrong:      "The pod reads and writes the node's own disk, so it outlives the pod and is shared with everything else on that node.",
			remedy:     "Use a PersistentVolumeClaim, an emptyDir or a ConfigMap instead.",
			find:       overSubjects(hostPathVolume),
		},
		{
			id:         "sensitive-host-path",
			title:      "Sensitive node path mounted",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{nsaCisa},
			wrong:      "These paths carry the node's identity, its credentials or its running state.",
			remedy:     "Remove the hostPath volume; read what you need from the API server instead.",
			find:       overSubjects(sensitiveHostPath),
		},
		{
			id:         "writable-host-mount",
			title:      "Node filesystem mounted writable",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{nsaCisa},
			wrong:      "The container can change the node's files, which persists after the pod is gone.",
			remedy:     "Set readOnly: true on the volumeMount, or drop the mount.",
			find:       overContainers(writableHostMount),
		},
		{
			id:         "capabilities-not-dropped",
			title:      "Capabilities not dropped",
			category:   categorySecurity,
			severity:   severityMedium,
			frameworks: []string{pssRestricted, nsaCisa},
			wrong:      "The container keeps the runtime's whole default capability set rather than only what it needs.",
			remedy:     "Set securityContext.capabilities.drop to [ALL], then add back what the process actually needs.",
			find:       overContainers(capabilitiesNotDropped),
		},
		{
			id:         "net-raw-kept",
			title:      "NET_RAW kept",
			category:   categorySecurity,
			severity:   severityMedium,
			frameworks: []string{nsaCisa},
			wrong:      "NET_RAW allows raw sockets, which is how a compromised container spoofs and sniffs traffic on its network.",
			remedy:     "Drop NET_RAW, or drop ALL.",
			find:       overContainers(netRawKept),
		},
		{
			id:         "restricted-volume-types",
			title:      "Volume type outside the restricted set",
			category:   categorySecurity,
			severity:   severityMedium,
			frameworks: []string{pssRestricted},
			wrong:      "The restricted profile allows only volume types the pod cannot use to reach the node or the network directly.",
			remedy:     "Move the data behind a PersistentVolumeClaim, a projected volume or a CSI driver.",
			find:       overSubjects(restrictedVolumes),
		},
		{
			id:         "root-group",
			title:      "Running with a root group",
			category:   categorySecurity,
			severity:   severityMedium,
			frameworks: []string{nsaCisa},
			wrong:      "Group 0 reaches files the root group owns, which is most of what a break-out wants.",
			remedy:     "Set runAsGroup, fsGroup and supplementalGroups to a non-zero id.",
			find:       overContainers(rootGroup),
		},
		{
			id:         "host-process",
			title:      "Windows HostProcess container",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{pssBaseline},
			wrong:      "A HostProcess container runs directly on the Windows node with the node's own privileges.",
			remedy:     "Set windowsOptions.hostProcess to false.",
			find:       overContainers(hostProcess),
		},
		{
			id:         "automount-token",
			arguable:   true,
			title:      "Service account token mounted",
			category:   categorySecurity,
			severity:   severityLow,
			frameworks: []string{nsaCisa},
			wrong:      "A token that reaches the API server is on disk in every container, whether or not anything reads it. A judgement call: leave it on for workloads that genuinely talk to the API server.",
			remedy:     "Set automountServiceAccountToken to false on the pod, or on the ServiceAccount it uses.",
			find:       overSubjects(automountToken),
		},
		{
			id:         "default-service-account",
			arguable:   true,
			title:      "Default service account used",
			category:   categorySecurity,
			severity:   severityLow,
			frameworks: []string{nsaCisa},
			wrong:      "Every workload that names no account shares one identity, so a grant to any of them is a grant to all. A judgement call on a cluster where the default account holds nothing.",
			remedy:     "Give the workload its own ServiceAccount and name it in serviceAccountName.",
			find:       overSubjects(defaultServiceAccount),
		},
	}
}

func profileType(spec map[string]any, field string) (value string, set bool) {
	profile, ok := spec[field].(map[string]any)
	if !ok {
		return "", false
	}
	return stringAt(profile, "type"), true
}

func seccompUnset(subject Subject, container Container) (string, string) {
	if _, set := profileType(securityContextOf(container.Spec), seccompField); set {
		return "", ""
	}
	if _, set := profileType(securityContextOf(subject.Pod), seccompField); set {
		return "", ""
	}
	body := []string{
		securityBlock,
		indent + seccompField + ":",
		strings.Repeat(indent, 2) + "type: " + runtimeDefault,
	}
	return "no securityContext.seccompProfile on the container or the pod", containerPatch(subject, container, body)
}

func seccompUnconfined(subject Subject, container Container) (string, string) {
	where, found := unconfinedProfile(subject, container, seccompField)
	if !found {
		return "", ""
	}
	body := []string{
		securityBlock,
		indent + seccompField + ":",
		strings.Repeat(indent, 2) + "type: " + runtimeDefault,
	}
	return "securityContext.seccompProfile.type is Unconfined on the " + where, containerPatch(subject, container, body)
}

func unconfinedProfile(subject Subject, container Container, field string) (where string, found bool) {
	if value, set := profileType(securityContextOf(container.Spec), field); set {
		if value == unconfined {
			return "container", true
		}
		return "", false
	}
	if value, set := profileType(securityContextOf(subject.Pod), field); set && value == unconfined {
		return "pod", true
	}
	return "", false
}

func apparmorUnconfined(subject Subject, container Container) (string, string) {
	if annotated := apparmorAnnotation(subject, container); annotated != "" {
		return annotated, ""
	}
	where, found := unconfinedProfile(subject, container, apparmorField)
	if !found {
		return "", ""
	}
	body := []string{
		securityBlock,
		indent + apparmorField + ":",
		strings.Repeat(indent, 2) + "type: " + runtimeDefault,
	}
	return "securityContext.appArmorProfile.type is Unconfined on the " + where, containerPatch(subject, container, body)
}

func apparmorAnnotation(subject Subject, container Container) string {
	annotations := subject.Object.GetAnnotations()
	value, ok := annotations[apparmorPrefix+container.Name]
	if !ok || !strings.EqualFold(value, "unconfined") {
		return ""
	}
	return "the " + apparmorPrefix + container.Name + " annotation is unconfined"
}

func seLinuxWidened(subject Subject, container Container) (string, string) {
	widened := narrowedFields(securityContextOf(container.Spec))
	if len(widened) == 0 {
		widened = narrowedFields(securityContextOf(subject.Pod))
	}
	if len(widened) == 0 {
		return "", ""
	}
	return "seLinuxOptions sets " + strings.Join(widened, ", "), ""
}

func narrowedFields(spec map[string]any) []string {
	options, ok := spec[seLinuxField].(map[string]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, field := range seLinuxNarrowing {
		if stringAt(options, field) != "" {
			out = append(out, field)
		}
	}
	return out
}

func procMountUnmasked(subject Subject, container Container) (string, string) {
	if stringAt(securityContextOf(container.Spec), procMountField) != procMountUnmask {
		return "", ""
	}
	body := []string{securityBlock, indent + procMountField + ": Default"}
	return "securityContext.procMount is Unmasked", containerPatch(subject, container, body)
}

func unsafeSysctls(subject Subject) (string, string) {
	named := listedSysctls(subject)
	if len(named) == 0 {
		return "", ""
	}
	return "sets the unsafe " + plural(named, "sysctl") + " " + strings.Join(named, ", "), ""
}

func listedSysctls(subject Subject) []string {
	listed, ok := securityContextOf(subject.Pod)[sysctlsField].([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, entry := range listed {
		item, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		name := stringAt(item, "name")
		if name == "" || slices.Contains(safeSysctls, name) {
			continue
		}
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func plural(named []string, word string) string {
	if len(named) == 1 {
		return word
	}
	return word + "s"
}

func hostPorts(subject Subject, container Container) (string, string) {
	claimed := claimedHostPorts(container)
	if len(claimed) == 0 {
		return "", ""
	}
	return "binds host " + plural(claimed, "port") + " " + strings.Join(claimed, ", "), ""
}

func claimedHostPorts(container Container) []string {
	listed, ok := container.Spec["ports"].([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, entry := range listed {
		item, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		port, set := numberAt(item, "hostPort")
		if !set || port == 0 {
			continue
		}
		out = append(out, strconv.FormatInt(port, 10))
	}
	return out
}

func hostPathVolume(subject Subject) (string, string) {
	name, path := firstHostPath(subject, false)
	if name == "" {
		return "", ""
	}
	return "volume " + name + " mounts " + path + " from the node", volumePatch(subject, name)
}

func sensitiveHostPath(subject Subject) (string, string) {
	name, path := firstHostPath(subject, true)
	if name == "" {
		return "", ""
	}
	return "volume " + name + " mounts " + path + " from the node", volumePatch(subject, name)
}

func firstHostPath(subject Subject, wantSensitive bool) (name, path string) {
	volumes, ok := subject.Pod["volumes"].([]any)
	if !ok {
		return "", ""
	}
	for _, entry := range volumes {
		volume, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		found := hostPathOf(volume)
		if found == "" || slices.Contains(runtimeSockets, found) {
			continue
		}
		if slices.Contains(sensitiveHostPaths, found) != wantSensitive {
			continue
		}
		return stringAt(volume, "name"), found
	}
	return "", ""
}

func hostPathOf(volume map[string]any) string {
	spec, ok := volume[sensitiveMount].(map[string]any)
	if !ok {
		return ""
	}
	return stringAt(spec, "path")
}

func writableHostMount(subject Subject, container Container) (string, string) {
	for _, name := range hostPathVolumeNames(subject) {
		if mounted := writableMountOf(container, name); mounted != "" {
			return "mounts the node's " + mounted + " writable", ""
		}
	}
	return "", ""
}

func hostPathVolumeNames(subject Subject) []string {
	volumes, ok := subject.Pod["volumes"].([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, entry := range volumes {
		volume, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		if hostPathOf(volume) == "" {
			continue
		}
		out = append(out, stringAt(volume, "name"))
	}
	return out
}

func writableMountOf(container Container, volume string) string {
	listed, ok := container.Spec["volumeMounts"].([]any)
	if !ok {
		return ""
	}
	for _, entry := range listed {
		item, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		if stringAt(item, "name") != volume {
			continue
		}
		readOnly, set := boolAt(item, "readOnly")
		if set && readOnly {
			continue
		}
		return stringAt(item, "mountPath")
	}
	return ""
}

func capabilitiesNotDropped(subject Subject, container Container) (string, string) {
	if slices.ContainsFunc(droppedCapabilities(container), func(name string) bool {
		return strings.EqualFold(name, dropAll)
	}) {
		return "", ""
	}
	body := []string{
		securityBlock,
		indent + "capabilities:",
		strings.Repeat(indent, 2) + "drop:",
		strings.Repeat(indent, 3) + "- " + dropAll,
	}
	return "securityContext.capabilities.drop does not include ALL", containerPatch(subject, container, body)
}

func netRawKept(subject Subject, container Container) (string, string) {
	dropped := droppedCapabilities(container)
	for _, name := range dropped {
		if strings.EqualFold(name, dropAll) || strings.EqualFold(name, netRaw) {
			return "", ""
		}
	}
	body := []string{
		securityBlock,
		indent + "capabilities:",
		strings.Repeat(indent, 2) + "drop:",
		strings.Repeat(indent, 3) + "- " + netRaw,
	}
	return "NET_RAW is not in securityContext.capabilities.drop", containerPatch(subject, container, body)
}

func droppedCapabilities(container Container) []string {
	capabilities, ok := securityContextOf(container.Spec)["capabilities"].(map[string]any)
	if !ok {
		return nil
	}
	listed, ok := capabilities["drop"].([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, entry := range listed {
		name, isString := entry.(string)
		if !isString {
			continue
		}
		out = append(out, name)
	}
	return out
}

func restrictedVolumes(subject Subject) (string, string) {
	named := disallowedVolumeTypes(subject)
	if len(named) == 0 {
		return "", ""
	}
	return "uses the " + plural(named, "volume type") + " " + strings.Join(named, ", "), ""
}

func disallowedVolumeTypes(subject Subject) []string {
	volumes, ok := subject.Pod["volumes"].([]any)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	out := []string{}
	for _, entry := range volumes {
		volume, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		for key := range volume {
			if key == "name" || slices.Contains(restrictedVolumeTypes, key) {
				continue
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, key)
		}
	}
	slices.Sort(out)
	return out
}

func rootGroup(subject Subject, container Container) (string, string) {
	own := securityContextOf(container.Spec)
	inherited := securityContextOf(subject.Pod)
	for _, field := range []string{"runAsGroup", "fsGroup"} {
		if rootAt(own, inherited, field) {
			return "securityContext." + field + " is 0", ""
		}
	}
	if rootSupplemental(own, inherited) {
		return "securityContext.supplementalGroups includes 0", ""
	}
	return "", ""
}

func rootAt(own, inherited map[string]any, field string) bool {
	value, set := numberAt(own, field)
	if !set {
		value, set = numberAt(inherited, field)
	}
	return set && value == 0
}

func rootSupplemental(own, inherited map[string]any) bool {
	listed, ok := own["supplementalGroups"].([]any)
	if !ok {
		listed, ok = inherited["supplementalGroups"].([]any)
	}
	if !ok {
		return false
	}
	for _, entry := range listed {
		switch value := entry.(type) {
		case int64:
			if value == 0 {
				return true
			}
		case float64:
			if value == 0 {
				return true
			}
		default:
			continue
		}
	}
	return false
}

func hostProcess(subject Subject, container Container) (string, string) {
	if !hostProcessAt(securityContextOf(container.Spec)) && !hostProcessAt(securityContextOf(subject.Pod)) {
		return "", ""
	}
	body := []string{
		securityBlock,
		indent + windowsOptions + ":",
		strings.Repeat(indent, 2) + hostProcessField + ": false",
	}
	return "windowsOptions.hostProcess is true", containerPatch(subject, container, body)
}

func hostProcessAt(spec map[string]any) bool {
	options, ok := spec[windowsOptions].(map[string]any)
	if !ok {
		return false
	}
	value, set := boolAt(options, hostProcessField)
	return set && value
}

func automountToken(subject Subject) (string, string) {
	value, set := boolAt(subject.Pod, "automountServiceAccountToken")
	if set && !value {
		return "", ""
	}
	detail := "automountServiceAccountToken is unset, which mounts the token"
	if set {
		detail = "automountServiceAccountToken is true"
	}
	return detail, podPatch(subject, []string{"automountServiceAccountToken: false"})
}

func defaultServiceAccount(subject Subject) (string, string) {
	name := stringAt(subject.Pod, "serviceAccountName")
	if name != "" && name != defaultNamespace {
		return "", ""
	}
	if name == "" {
		return "serviceAccountName is unset, so the pod uses the namespace's default account", ""
	}
	return "serviceAccountName is the namespace's default account", ""
}
