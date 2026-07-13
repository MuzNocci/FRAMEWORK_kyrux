package render

import (
	"bytes"
	"kyrux/core/router"
	"runtime"
	"strconv"
	"sync"
)

var goroutineCtxStore sync.Map

func gid() uint64 {
	var buf [32]byte
	n := runtime.Stack(buf[:], false)
	s := buf[10:n] // skip "goroutine "
	i := bytes.IndexByte(s, ' ')
	id, _ := strconv.ParseUint(string(s[:i]), 10, 64)
	return id
}

// setCurrentCtx devolve o id da goroutine para que clearCurrentCtx o reuse —
// gid() custa ~2 µs (runtime.Stack); computar uma vez por render, não duas.
func setCurrentCtx(ctx *router.Context) uint64 {
	id := gid()
	goroutineCtxStore.Store(id, ctx)
	return id
}

func clearCurrentCtx(id uint64) { goroutineCtxStore.Delete(id) }

// GetCurrentCtx retorna o *router.Context da goroutine em execução.
// Usado por funções do FuncMap que precisam de dados por request.
func GetCurrentCtx() *router.Context {
	if v, ok := goroutineCtxStore.Load(gid()); ok {
		return v.(*router.Context)
	}
	return nil
}

// AddFunc registra uma função no FuncMap global de templates.
// Deve ser chamado antes do primeiro render.
func AddFunc(name string, fn any) {
	templateFuncs[name] = fn
}
