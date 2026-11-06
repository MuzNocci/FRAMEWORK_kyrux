package admin

import (
	"net/http/httptest"
	"strings"
	"testing"

	"kyrux/core/security/auth"
)

// A tela de login do admin acompanha AUTH_LOGIN_FIELD.
func TestLoginPageSegueCampoDeLogin(t *testing.T) {
	t.Cleanup(func() { auth.SetLoginField("username") })
	casos := []struct{ campo, label, tipo, erro string }{
		{"username", "Usuário", `type="text" id="f-login"`, "Usuário ou senha inválidos."},
		{"email", "E-mail", `type="email" id="f-login"`, "E-mail ou senha inválidos."},
	}
	for _, c := range casos {
		if err := auth.SetLoginField(c.campo); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		renderPage(w, loginTpl, newLoginPage(baseData{}, ""))
		corpo := w.Body.String()
		if !strings.Contains(corpo, `<label for="f-login">`+c.label+`</label>`) {
			t.Errorf("%s: rótulo %q ausente", c.campo, c.label)
		}
		if !strings.Contains(corpo, c.tipo) {
			t.Errorf("%s: campo %q ausente", c.campo, c.tipo)
		}
		if got := loginErrorMessage(auth.ErrWrongPassword); got != c.erro {
			t.Errorf("%s: mensagem %q", c.campo, got)
		}
	}
}
