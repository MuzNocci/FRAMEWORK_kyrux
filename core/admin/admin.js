document.addEventListener('DOMContentLoaded', function () {
  // Avatar do usuário: clique abre/fecha o dropdown; clique fora ou Esc
  // fecha. stopPropagation no botão evita que o próprio clique de abrir
  // já dispare o listener de "clique fora" no mesmo evento.
  var avatarBtn = document.querySelector('.js-admin-avatar-btn');
  var avatarDropdown = document.querySelector('.js-admin-avatar-dropdown');
  if (avatarBtn && avatarDropdown) {
    function closeAvatarMenu() {
      avatarDropdown.classList.remove('active');
      avatarBtn.setAttribute('aria-expanded', 'false');
    }
    avatarBtn.addEventListener('click', function (e) {
      e.stopPropagation();
      var open = avatarDropdown.classList.toggle('active');
      avatarBtn.setAttribute('aria-expanded', open ? 'true' : 'false');
    });
    document.addEventListener('click', function (e) {
      if (!avatarDropdown.contains(e.target)) closeAvatarMenu();
    });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') closeAvatarMenu();
    });
  }

  document.querySelectorAll('.js-confirm-delete').forEach(function (f) {
    f.addEventListener('submit', function (e) {
      if (!confirm('Confirma a exclusão deste registro? Esta ação não pode ser desfeita.')) {
        e.preventDefault();
      }
    });
  });

  // Seleção em lote: "selecionar todos" no cabeçalho, barra de ação some/
  // aparece conforme há linhas marcadas, e o botão Aplicar confirma antes
  // de submeter o form oculto (#bulk-form) — os checkboxes de cada linha
  // pertencem a ele via atributo form="bulk-form", não por aninhamento
  // (uma <table> não pode conter um <form> por linha e outro por fora).
  var bulkForm = document.getElementById('bulk-form');
  if (bulkForm) {
    var selectAll = document.querySelector('.js-select-all');
    var rowChecks = document.querySelectorAll('.js-row-check');
    var bar = document.getElementById('bulk-bar');
    var countEl = document.getElementById('bulk-count-num');
    var applyBtn = document.getElementById('bulk-apply-btn');
    var actionSelect = document.getElementById('bulk-action-select');
    var actionInput = document.getElementById('bulk-action-input');

    function updateBulkBar() {
      var checked = document.querySelectorAll('.js-row-check:checked').length;
      if (bar) bar.hidden = checked === 0;
      if (countEl) countEl.textContent = checked;
      if (selectAll) {
        selectAll.checked = checked > 0 && checked === rowChecks.length;
      }
    }
    if (selectAll) {
      selectAll.addEventListener('change', function () {
        rowChecks.forEach(function (c) { c.checked = selectAll.checked; });
        updateBulkBar();
      });
    }
    rowChecks.forEach(function (c) {
      c.addEventListener('change', updateBulkBar);
    });
    if (applyBtn && actionSelect && actionInput) {
      applyBtn.addEventListener('click', function () {
        var checked = document.querySelectorAll('.js-row-check:checked').length;
        if (checked === 0) return;
        var label = actionSelect.options[actionSelect.selectedIndex].text;
        if (!confirm('Confirma "' + label + '" em ' + checked + ' registro(s)? Esta ação pode não poder ser desfeita.')) {
          return;
        }
        actionInput.value = actionSelect.value;
        bulkForm.submit();
      });
    }
  }

  // Menu mobile: sidebar vira um drawer off-canvas abaixo de 800px, aberto
  // pelo botão hamburguer e fechado pelo overlay, tecla Esc, ou ao navegar
  // (clique em qualquer link dentro do drawer).
  var menuBtn = document.querySelector('.js-admin-menu-btn');
  var sidebar = document.getElementById('admin-sidebar');
  var overlay = document.querySelector('.js-admin-sidebar-overlay');
  if (menuBtn && sidebar && overlay) {
    function closeMenu() {
      sidebar.classList.remove('active');
      overlay.classList.remove('active');
      menuBtn.setAttribute('aria-expanded', 'false');
    }
    function openMenu() {
      sidebar.classList.add('active');
      overlay.classList.add('active');
      menuBtn.setAttribute('aria-expanded', 'true');
    }
    menuBtn.addEventListener('click', function () {
      if (sidebar.classList.contains('active')) closeMenu(); else openMenu();
    });
    overlay.addEventListener('click', closeMenu);
    sidebar.querySelectorAll('a').forEach(function (a) {
      a.addEventListener('click', closeMenu);
    });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') closeMenu();
    });
  }

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
