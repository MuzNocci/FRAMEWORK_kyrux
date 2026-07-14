document.addEventListener('DOMContentLoaded', function () {
  document.querySelectorAll('.js-confirm-delete').forEach(function (f) {
    f.addEventListener('submit', function (e) {
      if (!confirm('Confirma a exclusão deste registro? Esta ação não pode ser desfeita.')) {
        e.preventDefault();
      }
    });
  });
});
