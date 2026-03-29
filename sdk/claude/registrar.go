package claude

import "github.com/777genius/plugin-kit-ai/sdk/internal/runtime"

type Registrar struct {
	backend runtime.RegistrarBackend
}

func NewRegistrar(backend runtime.RegistrarBackend) *Registrar {
	return &Registrar{backend: backend}
}
