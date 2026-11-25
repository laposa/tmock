// tmock UI script (spec §10.1, §10.2).
//
// Loaded with `defer` after htmx and hx-alpine-compat, and before Alpine, so the
// components below are registered before Alpine starts. Alpine is the CSP build:
// components are registered here with Alpine.data(), and the HTML refers to them only by
// name and to their properties and methods by simple reference.
//
// htmx configuration is NOT set here: htmx 4 reads <meta name="htmx-config"> while its
// script loads, which is too early for this file. See internal/views/layout.
//
// This file sets no classes: components own their class names (spec §10.7). State that
// CSS needs is exposed as data attributes (data-type on a toast).

const TOAST_TIMEOUT_MS = 5000;

document.addEventListener('alpine:init', () => {
  // ── Toasts ────────────────────────────────────────────────────────────────
  // Handlers raise them with HX-Trigger: {"toast":{"type","title","message"}}; htmx 4
  // dispatches a "toast" event whose detail is that object (internal/web toast()).
  let nextToastID = 1;
  Alpine.store('toasts', {
    items: [],
    add({ type = 'info', title = '', message = '' } = {}) {
      const id = nextToastID++;
      const dismiss = () => this.remove(id);
      this.items.push({ id, type: String(type), title: String(title), message: String(message), dismiss });
      setTimeout(dismiss, TOAST_TIMEOUT_MS);
    },
    remove(id) {
      this.items = this.items.filter((t) => t.id !== id);
    },
  });

  // The toast region (components.ToastRegion) renders the store.
  Alpine.data('toasts', () => ({
    get items() {
      return Alpine.store('toasts').items;
    },
  }));

  document.addEventListener('toast', (e) => Alpine.store('toasts').add(e.detail));

  // A network failure or a timeout: there is no response, so no toast from the server.
  document.addEventListener('htmx:error', () => {
    Alpine.store('toasts').add({
      type: 'error',
      title: 'Connection problem',
      message: 'The server could not be reached. Please try again.',
    });
  });

  // ── Modal host (components.ModalHost) ─────────────────────────────────────
  // htmx loads content into #modal-body; the dialog opens when it arrives and is emptied
  // when it closes. Content with data-persistent (an editor with unsaved changes) does not
  // close on a click on the backdrop. A response can close the dialog with
  // HX-Trigger: {"modal:close":{"target":"body"}}.
  Alpine.data('modal', () => ({
    init() {
      const dialog = this.$el;
      document.addEventListener('htmx:after:settle', (e) => {
        if (dialog.contains(e.target) && !dialog.open && dialog.querySelector('#modal-body').hasChildNodes()) {
          dialog.showModal();
        }
      });
      document.addEventListener('modal:close', () => dialog.close());
      dialog.addEventListener('click', (e) => {
        if (e.target === dialog && !dialog.querySelector('[data-persistent]')) {
          dialog.close();
        }
      });
      dialog.addEventListener('close', () => {
        dialog.querySelector('#modal-body').replaceChildren();
      });
    },
  }));

  // ── Confirmation dialog (components.ConfirmDialog) ────────────────────────
  // Replaces window.confirm for hx-confirm: htmx 4 waits for issueRequest() or
  // dropRequest() when the htmx:confirm event is cancelled.
  Alpine.data('confirm', () => ({
    message: '',
    pending: null,
    init() {
      document.addEventListener('htmx:confirm', (e) => {
        const question = e.detail.ctx.confirm;
        if (!question) return;
        e.preventDefault();
        this.pending?.dropRequest();
        this.pending = e.detail;
        this.message = question;
        this.$el.showModal();
      });
      this.$el.addEventListener('close', () => {
        this.pending?.dropRequest(); // Escape or Cancel
        this.pending = null;
      });
    },
    // $root, not $el: in an x-on handler $el is the button that was clicked.
    accept() {
      const pending = this.pending;
      this.pending = null;
      this.$root.close();
      pending?.issueRequest();
    },
    cancel() {
      this.$root.close();
    },
  }));

  // ── Forms ─────────────────────────────────────────────────────────────────
  // Keeps the submit button disabled while a required field is empty (spec §10.3 Login,
  // A2). Use: x-data="requiredFields" x-on:input="update" on the form, and
  // x-bind:disabled="incomplete" on the button. Without JavaScript the button stays
  // enabled and the server validates.
  Alpine.data('requiredFields', () => ({
    incomplete: false,
    init() {
      this.update();
    },
    update() {
      this.incomplete = [...this.$root.querySelectorAll('[required]')].some((el) => el.value.trim() === '');
    },
  }));

  // Shows on placeholder pages that both libraries are running (I00 verification).
  Alpine.data('jsStatus', () => ({
    message: `htmx ${htmx.version} and Alpine ${Alpine.version} are running.`,
  }));
});

document.addEventListener('alpine:initialized', () => {
  console.info(`tmock: htmx ${htmx.version}, Alpine ${Alpine.version}`);
});
