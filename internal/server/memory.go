package server

import (
	"net/http"
	"runtime"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const bytesPerMi = 1024 * 1024

func handleMemory(w http.ResponseWriter, _ *http.Request) {
	var held runtime.MemStats
	runtime.ReadMemStats(&held)
	writeJSON(w, api.Memory{
		HeapMi: int64(held.HeapAlloc / bytesPerMi),
		SysMi:  int64(held.Sys / bytesPerMi),
	})
}
