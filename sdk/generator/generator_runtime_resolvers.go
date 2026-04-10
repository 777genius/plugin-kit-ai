package generator

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
)

func renderResolvers(m model) string {
	var b strings.Builder
	b.WriteString("package gen\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"strings\"\n")
	b.WriteString("\t\"github.com/777genius/plugin-kit-ai/sdk/internal/runtime\"\n")
	b.WriteString(")\n\n")
	b.WriteString("func ResolveInvocation(args []string, _ runtime.Env) (runtime.Invocation, error) {\n")
	b.WriteString("\tif len(args) < 2 {\n")
	b.WriteString("\t\treturn runtime.Invocation{}, fmt.Errorf(\"usage: <binary> <hookName>\")\n")
	b.WriteString("\t}\n")
	b.WriteString("\traw := args[1]\n")
	for _, e := range m.events {
		switch e.Invocation.Kind {
		case runtime.InvocationArgvCommand:
			b.WriteString(fmt.Sprintf("\tif raw == %q {\n", e.Invocation.Name))
		case runtime.InvocationArgvCommandCaseFold:
			b.WriteString(fmt.Sprintf("\tif strings.EqualFold(raw, %q) {\n", e.Invocation.Name))
		case runtime.InvocationCustomResolver:
			continue
		}
		b.WriteString(fmt.Sprintf("\t\treturn runtime.Invocation{Platform: %q, Event: %q, RawName: raw}, nil\n", e.Platform, e.Event))
		b.WriteString("\t}\n")
	}
	b.WriteString("\treturn runtime.Invocation{}, fmt.Errorf(\"unknown invocation %q\", raw)\n")
	b.WriteString("}\n")
	return b.String()
}
