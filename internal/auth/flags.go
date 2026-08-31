package auth

import "context"

func KubectlFlags(ctx context.Context) []string {
	return flagsFor(ctx, "--as", "--as-group")
}

func HelmFlags(ctx context.Context) []string {
	return flagsFor(ctx, "--kube-as-user", "--kube-as-group")
}

func flagsFor(ctx context.Context, user, group string) []string {
	who, ok := ActingAs(ctx)
	if !ok {
		return nil
	}
	out := []string{user, who.User}
	for _, one := range who.Groups {
		out = append(out, group, one)
	}
	return out
}
