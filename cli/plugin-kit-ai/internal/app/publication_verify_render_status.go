package app

import "fmt"

type publicationVerifyStatus struct {
	ready     bool
	label     string
	nextSteps []string
}

func buildPublicationVerifyRootStatus(ctx publicationContext, plan publicationVerifyPlan) publicationVerifyStatus {
	if len(plan.issues) == 0 {
		return publicationVerifyStatus{
			ready: true,
			label: "ready",
		}
	}
	return publicationVerifyStatus{
		ready: false,
		label: "needs_sync",
		nextSteps: []string{
			fmt.Sprintf("run plugin-kit-ai publication materialize %s --target %s --dest %s", ctx.root, ctx.target, ctx.dest),
		},
	}
}
