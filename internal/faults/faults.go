package faults

import (
	"regexp"
	"strings"
)

type pattern struct {
	match *regexp.Regexp
	cause string
}

var causes = []pattern{
	{
		regexp.MustCompile(`(?i)metadata\.annotations: Too long|annotation is too large|exceeds the maximum`),
		"the manifest is too large for the last-applied annotation; sync with server-side apply",
	},
	{
		regexp.MustCompile(`(?i)admission webhook "([^"]+)" denied the request`),
		"the admission webhook $1 rejected it",
	},
	{
		regexp.MustCompile(`(?i)field is immutable|may not be changed|cannot change`),
		"an immutable field changed; sync with replace to recreate it",
	},
	{
		regexp.MustCompile(`(?i)is forbidden: User "([^"]+)" cannot`),
		"$1 may not write that resource",
	},
	{
		regexp.MustCompile(`(?i)is forbidden|forbidden:`),
		"argo cd may not write that resource",
	},
	{
		regexp.MustCompile(`(?i)Operation terminated`),
		"the operation was stopped",
	},
	{
		regexp.MustCompile(`(?i)namespaces "([^"]+)" not found`),
		"the destination namespace $1 does not exist; add CreateNamespace=true or create it in git",
	},
	{
		regexp.MustCompile(`(?i)error validating data|ValidationError`),
		"the manifest does not match the cluster's schema for that kind",
	},
	{
		regexp.MustCompile(`(?i)no matches for kind "([^"]+)"|the server could not find the requested resource`),
		"the crd for that kind is missing or not ready",
	},
	{
		regexp.MustCompile(`(?i)authentication required|could not read Username|invalid credentials|401 Unauthorized`),
		"the repository refused argo cd's credentials",
	},
	{
		regexp.MustCompile(`(?i)repository not found|remote: Repository not found|unable to resolve|couldn't find remote ref`),
		"the repository or revision could not be resolved",
	},
	{
		regexp.MustCompile(`(?i)context deadline exceeded|dial tcp|connection refused|i/o timeout|TLS handshake`),
		"argo cd could not reach the cluster or the repository",
	},
	{
		regexp.MustCompile(`(?i)hook.*[Ff]ailed|SyncFailed`),
		"a sync hook failed",
	},
	{
		regexp.MustCompile(`(?i)another operation is already in progress`),
		"another operation is still running",
	},
	{
		regexp.MustCompile(`(?i)Timed out waiting|health check.*timed out|progressing deadline`),
		"the resources were applied but never became healthy",
	},
	{
		regexp.MustCompile(`(?i)Resource .* not permitted in project|is not permitted in project`),
		"the appproject does not allow that resource",
	},
	{
		regexp.MustCompile(`(?i)Operation cannot be fulfilled|the object has been modified`),
		"something else changed the resource while argo cd was writing it",
	},
}

func Cause(message string) string {
	if message == "" {
		return ""
	}
	for _, one := range causes {
		found := one.match.FindStringSubmatchIndex(message)
		if found == nil {
			continue
		}
		return strings.TrimSpace(string(one.match.ExpandString(nil, one.cause, message, found)))
	}
	return ""
}
