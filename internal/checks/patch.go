package checks

import (
	"slices"
	"strings"
)

const indent = "  "

func nest(path, lines []string) string {
	out := make([]string, 0, len(path)+len(lines))
	for depth, key := range path {
		out = append(out, strings.Repeat(indent, depth)+key+":")
	}
	prefix := strings.Repeat(indent, len(path))
	for _, line := range lines {
		out = append(out, prefix+line)
	}
	return strings.Join(out, "\n") + "\n"
}

func containerListName(init bool) string {
	if init {
		return "initContainers"
	}
	return "containers"
}

func containerList(container Container) string {
	return containerListName(container.Init)
}

func volumeMountPaths(container Container, volume string) []string {
	listed, ok := container.Spec["volumeMounts"].([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, entry := range listed {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if stringAt(item, "name") != volume {
			continue
		}
		path := stringAt(item, "mountPath")
		if path == "" {
			continue
		}
		out = append(out, path)
	}
	return out
}

func mountRemovals(subject Subject, volume string, init bool) []string {
	out := []string{}
	for _, container := range subject.Containers {
		if container.Init != init {
			continue
		}
		paths := volumeMountPaths(container, volume)
		if len(paths) == 0 {
			continue
		}
		if len(out) == 0 {
			out = append(out, containerListName(init)+":")
		}
		out = append(out,
			indent+"- name: "+container.Name,
			strings.Repeat(indent, 2)+"volumeMounts:")
		for _, path := range paths {
			out = append(out,
				strings.Repeat(indent, 3)+"- mountPath: "+path,
				strings.Repeat(indent, 4)+"$patch: delete")
		}
	}
	return out
}

func volumePatch(subject Subject, volume string) string {
	body := mountRemovals(subject, volume, false)
	body = append(body, mountRemovals(subject, volume, true)...)
	body = append(body,
		"volumes:",
		indent+"- name: "+volume,
		strings.Repeat(indent, 2)+"$patch: delete")
	return podPatch(subject, body)
}

func containerPatch(subject Subject, container Container, body []string) string {
	lines := make([]string, 0, 2+len(body))
	lines = append(lines, containerList(container)+":", indent+"- name: "+container.Name)
	for _, line := range body {
		lines = append(lines, strings.Repeat(indent, 2)+line)
	}
	return nest(templatePath(subject.Kind), lines)
}

func podPatch(subject Subject, body []string) string {
	return nest(templatePath(subject.Kind), body)
}

func specPatch(body []string) string {
	return nest([]string{specField}, body)
}

func matchLabels(subject Subject) []string {
	labels, ok := subject.Object.Object[specField].(map[string]any)
	if !ok {
		return nil
	}
	selector, ok := labels["selector"].(map[string]any)
	if !ok {
		return nil
	}
	pairs, ok := selector["matchLabels"].(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		value, ok := pairs[key].(string)
		if !ok {
			continue
		}
		out = append(out, key+": "+value)
	}
	return out
}

func spreadPatch(subject Subject) string {
	body := []string{
		"topologySpreadConstraints:",
		indent + "- maxSkew: 1",
		strings.Repeat(indent, 2) + "topologyKey: kubernetes.io/hostname",
		strings.Repeat(indent, 2) + "whenUnsatisfiable: DoNotSchedule",
	}
	labels := matchLabels(subject)
	if len(labels) == 0 {
		return podPatch(subject, body)
	}
	body = append(body,
		strings.Repeat(indent, 2)+"labelSelector:",
		strings.Repeat(indent, 3)+"matchLabels:")
	for _, pair := range labels {
		body = append(body, strings.Repeat(indent, 4)+pair)
	}
	return podPatch(subject, body)
}
