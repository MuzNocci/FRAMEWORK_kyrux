document.addEventListener('DOMContentLoaded', function () {
  document.querySelectorAll('.js-confirm-delete').forEach(function (f) {
    f.addEventListener('submit', function (e) {
      if (!confirm('Confirma a exclusão deste registro? Esta ação não pode ser desfeita.')) {
        e.preventDefault();
      }
    });
  });
});

// Cache-Control: no-store (já enviado pelo servidor) nem sempre impede o
// bfcache do navegador de restaurar uma página autenticada ao clicar
// "voltar" sem fazer uma nova requisição — nesse caso o servidor nunca tem
// a chance de checar se a sessão ainda é válida (ex: após logout). Forçamos
// um reload real sempre que a página volta do bfcache, o que garante uma
// requisição nova e a checagem de sessão correta.
window.addEventListener('pageshow', function (e) {
  if (e.persisted) {
    location.reload();
  }
});
