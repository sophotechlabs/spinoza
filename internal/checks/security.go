package checks

import (
	"slices"
	"strings"
)

var hostNamespaceFields = []string{"hostPID", "hostIPC", "hostNetwork"}

var runtimeSockets = []string{
	"/run/containerd/containerd.sock",
	"/run/cri-dockerd.sock",
	"/run/crio/crio.sock",
	"/run/docker.sock",
	"/var/run/containerd/containerd.sock",
	"/var/run/cri-dockerd.sock",
	"/var/run/crio/crio.sock",
	"/var/run/docker.sock",
}

const (
	allowedCapability = "NET_BIND_SERVICE"
	securityBlock     = "securityContext:"
)

func securityContextOf(spec map[string]any) map[string]any {
	found, ok := spec["securityContext"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return found
}

func boolAt(spec map[string]any, key string) (value, set bool) {
	found, ok := spec[key].(bool)
	return found, ok
}

func securityChecks() []check {
	return []check{
		{
			id:         "privileged-containers",
			title:      "Privileged containers",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{pssBaseline, nsaCisa},
			wrong:      "A privileged container holds every capability on the node and can reach the host's devices, so escaping it is escaping the node.",
			remedy:     "Remove securityContext.privileged and add back only the capabilities the process actually needs.",
			find:       overContainers(privileged),
		},
		{
			id:         "privilege-escalation",
			title:      "Privilege escalation allowed",
			category:   categorySecurity,
			severity:   severityMedium,
			frameworks: []string{pssRestricted, nsaCisa},
			wrong:      "A process in the container can gain more privileges than its parent, which is how a setuid binary turns a shell into root.",
			remedy:     "Set securityContext.allowPrivilegeEscalation to false. It defaults to true when left unset.",
			find:       overContainers(escalation),
		},
		{
			id:         "host-namespaces",
			title:      "Host namespaces shared",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{pssBaseline, nsaCisa},
			wrong:      "The pod sees the node's processes, IPC or network directly, so the isolation between it and everything else on that node is gone.",
			remedy:     "Set hostPID, hostIPC and hostNetwork to false. Use a Service for connectivity instead of the host's network.",
			find:       overSubjects(hostNamespaces),
		},
		{
			id:         "runtime-socket-mounted",
			title:      "Container runtime socket mounted",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{nsaCisa},
			wrong:      "The container runtime's socket is the runtime's full API, so anything holding it can start a privileged container on the node.",
			remedy:     "Remove the hostPath volume. Where the workload needs cluster state, give it a ServiceAccount and read the API server.",
			find:       overSubjects(runtimeSocket),
		},
		{
			id:         "run-as-root",
			title:      "Containers running as root",
			category:   categorySecurity,
			severity:   severityMedium,
			frameworks: []string{pssRestricted, nsaCisa},
			wrong:      "The process runs as uid 0 inside the container, so a break-out lands on the node as root.",
			remedy:     "Set securityContext.runAsNonRoot to true and give the image a non-zero user.",
			find:       overContainers(rootUser),
		},
		{
			id:         "writable-root-filesystem",
			title:      "Writable root filesystem",
			category:   categorySecurity,
			severity:   severityMedium,
			frameworks: []string{nsaCisa},
			wrong:      "The container can rewrite its own binaries, so anything that gets code execution can also persist it.",
			remedy:     "Set securityContext.readOnlyRootFilesystem to true and mount an emptyDir wherever the process needs to write.",
			find:       overContainers(writableRoot),
		},
		{
			id:         "dangerous-capabilities",
			title:      "Dangerous capabilities added",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{pssBaseline, nsaCisa},
			wrong:      "Capabilities beyond the baseline set hand the container parts of root on the node: SYS_ADMIN alone covers mounting and most of what privileged does.",
			remedy:     "Drop ALL, then add back only what the process needs. Only NET_BIND_SERVICE is within the baseline.",
			find:       overContainers(dangerousCapabilities),
		},
	}
}

func privileged(subject Subject, container Container) (string, string) {
	value, set := boolAt(securityContextOf(container.Spec), "privileged")
	if !set || !value {
		return "", ""
	}
	body := []string{securityBlock, indent + "privileged: false"}
	return "securityContext.privileged is true", containerPatch(subject, container, body)
}

func escalation(subject Subject, container Container) (string, string) {
	value, set := boolAt(securityContextOf(container.Spec), "allowPrivilegeEscalation")
	if set && !value {
		return "", ""
	}
	detail := "securityContext.allowPrivilegeEscalation is unset, which defaults to true"
	if set {
		detail = "securityContext.allowPrivilegeEscalation is true"
	}
	body := []string{securityBlock, indent + "allowPrivilegeEscalation: false"}
	return detail, containerPatch(subject, container, body)
}

func hostNamespaces(subject Subject) (string, string) {
	shared := []string{}
	for _, field := range hostNamespaceFields {
		value, set := boolAt(subject.Pod, field)
		if set && value {
			shared = append(shared, field)
		}
	}
	if len(shared) == 0 {
		return "", ""
	}
	body := make([]string, 0, len(shared))
	for _, field := range shared {
		body = append(body, field+": false")
	}
	return "shares the host namespaces: " + strings.Join(shared, ", "), podPatch(subject, body)
}

func runtimeSocket(subject Subject) (string, string) {
	name, path := mountedSocket(subject)
	if path == "" {
		return "", ""
	}
	body := []string{"volumes:", indent + "- name: " + name, strings.Repeat(indent, 2) + "$patch: delete"}
	return "volume " + name + " mounts " + path + " from the node", podPatch(subject, body)
}

func mountedSocket(subject Subject) (name, path string) {
	volumes, ok := subject.Pod["volumes"].([]any)
	if !ok {
		return "", ""
	}
	for _, entry := range volumes {
		volume, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hostPath, ok := volume["hostPath"].(map[string]any)
		if !ok {
			continue
		}
		found, ok := hostPath["path"].(string)
		if !ok {
			continue
		}
		if slices.Contains(runtimeSockets, found) {
			return stringAt(volume, "name"), found
		}
	}
	return "", ""
}

func stringAt(entry map[string]any, key string) string {
	value, ok := entry[key].(string)
	if !ok {
		return ""
	}
	return value
}

func rootUser(subject Subject, container Container) (string, string) {
	detail := rootDetail(subject, container)
	if detail == "" {
		return "", ""
	}
	body := []string{securityBlock, indent + "runAsNonRoot: true"}
	return detail, containerPatch(subject, container, body)
}

func rootDetail(subject Subject, container Container) string {
	own := securityContextOf(container.Spec)
	inherited := securityContextOf(subject.Pod)
	user, hasUser := userIDOf(own, inherited)
	if hasUser && user == 0 {
		return "securityContext.runAsUser is 0"
	}
	refused, stated := nonRoot(own, inherited)
	if stated && refused {
		return ""
	}
	if hasUser {
		return ""
	}
	if stated {
		return "securityContext.runAsNonRoot is false, so the container may run as root"
	}
	return "securityContext.runAsNonRoot is unset, so the container runs as whatever user the image names"
}

func userIDOf(own, inherited map[string]any) (value int64, set bool) {
	found, ok := numberAt(own, "runAsUser")
	if ok {
		return found, true
	}
	return numberAt(inherited, "runAsUser")
}

func nonRoot(own, inherited map[string]any) (value, stated bool) {
	found, set := boolAt(own, "runAsNonRoot")
	if set {
		return found, true
	}
	return boolAt(inherited, "runAsNonRoot")
}

func numberAt(spec map[string]any, key string) (value int64, set bool) {
	switch found := spec[key].(type) {
	case int64:
		return found, true
	case float64:
		return int64(found), true
	default:
		return 0, false
	}
}

func writableRoot(subject Subject, container Container) (string, string) {
	value, set := boolAt(securityContextOf(container.Spec), "readOnlyRootFilesystem")
	if set && value {
		return "", ""
	}
	detail := "securityContext.readOnlyRootFilesystem is unset, so the root filesystem is writable"
	if set {
		detail = "securityContext.readOnlyRootFilesystem is false"
	}
	body := []string{securityBlock, indent + "readOnlyRootFilesystem: true"}
	return detail, containerPatch(subject, container, body)
}

func dangerousCapabilities(subject Subject, container Container) (string, string) {
	added := addedCapabilities(container)
	if len(added) == 0 {
		return "", ""
	}
	body := []string{
		securityBlock,
		indent + "capabilities:",
		strings.Repeat(indent, 2) + "add: []",
		strings.Repeat(indent, 2) + "drop:",
		strings.Repeat(indent, 3) + "- ALL",
	}
	detail := "securityContext.capabilities.add carries " + strings.Join(added, ", ")
	return detail, containerPatch(subject, container, body)
}

func addedCapabilities(container Container) []string {
	capabilities, ok := securityContextOf(container.Spec)["capabilities"].(map[string]any)
	if !ok {
		return nil
	}
	listed, ok := capabilities["add"].([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, entry := range listed {
		name, ok := entry.(string)
		if !ok {
			continue
		}
		if strings.EqualFold(name, allowedCapability) {
			continue
		}
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}
