package bootstrap

import "kyrux/core/router"

type RegisterFunc func(r *router.Router, fw *Framework)

var registry = map[string]RegisterFunc{}

func RegisterApp(name string, fn RegisterFunc) {
	registry[name] = fn
}
