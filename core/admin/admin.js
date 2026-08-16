document.addEventListener('DOMContentLoaded', function () {
  document.querySelectorAll('.js-confirm-delete').forEach(function (f) {
    f.addEventListener('submit', function (e) {
      if (!confirm('Confirma a exclusão deste registro? Esta ação não pode ser desfeita.')) {
        e.preventDefault();
      }
    });
  });

  // Pré-visualização do campo de imagem (kyrux:"image") assim que um
  // arquivo é escolhido — antes do upload, só lendo o arquivo local.
  document.querySelectorAll('.js-image-input').forEach(function (input) {
    input.addEventListener('change', function () {
      var file = input.files && input.files[0];
      if (!file) return;
      var preview = document.getElementById('preview-' + input.name);
      if (!preview) {
        preview = document.createElement('img');
        preview.id = 'preview-' + input.name;
        preview.className = 'admin-image-preview';
        preview.alt = '';
        input.insertAdjacentElement('beforebegin', preview);
      }
      var reader = new FileReader();
      reader.onload = function (e) { preview.src = e.target.result; };
      reader.readAsDataURL(file);
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
