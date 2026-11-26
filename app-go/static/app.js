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
  // HX-Trigger: {"modal:close":{"target":"body"}}, and its Close button with close().
  Alpine.data('modal', () => ({
    init() {
      const dialog = this.$el;
      document.addEventListener('htmx:after:settle', (e) => {
        if (dialog.contains(e.target) && !dialog.open && dialog.querySelector('#modal-body').hasChildNodes()) {
          dialog.showModal();
          // "Show Headers" / "Show Body" open the scenario editor scrolled to that part.
          const id = dialog.querySelector('[data-scroll-to]')?.dataset.scrollTo;
          if (id) document.getElementById(id)?.scrollIntoView({ block: 'start' });
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
    // The Close button of modal content (components.CloseModalButton). $root, not $el:
    // in an x-on handler $el is the button that was clicked.
    close() {
      this.$root.close();
    },
  }));

  // ── Confirmation dialog (components.ConfirmDialog) ────────────────────────
  // Replaces window.confirm for hx-confirm: htmx 4 waits for issueRequest() or
  // dropRequest() when the htmx:confirm event is cancelled. Other components ask with
  // a "confirm:ask" event on document, detail {question, accept}; accept() runs on
  // Confirm.
  Alpine.data('confirm', () => ({
    message: '',
    pending: null,
    init() {
      const ask = (question, pending) => {
        this.pending?.dropRequest();
        this.pending = pending;
        this.message = question;
        this.$el.showModal();
      };
      document.addEventListener('htmx:confirm', (e) => {
        const question = e.detail.ctx.confirm;
        if (!question) return;
        e.preventDefault();
        ask(question, e.detail);
      });
      document.addEventListener('confirm:ask', (e) => {
        ask(e.detail.question, { issueRequest: e.detail.accept, dropRequest() {} });
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

  // ── Popover (components.Popover) ──────────────────────────────────────────
  // Shows the panel beside its trigger while it is hovered or focused. The panel is a
  // native popover in the top layer, placed with fixed coordinates (CSSOM, which the
  // CSP allows), on the right of the trigger or, without room, on its left.
  Alpine.data('popover', () => ({
    show() {
      const panel = this.$root.querySelector('[popover]');
      if (!panel || panel.matches(':popover-open')) return;
      panel.showPopover();
      const gap = 8;
      const anchor = this.$root.getBoundingClientRect();
      const box = panel.getBoundingClientRect();
      let left = anchor.right + gap;
      if (left + box.width > window.innerWidth - gap) left = Math.max(gap, anchor.left - box.width - gap);
      const top = Math.max(gap, Math.min(anchor.top, window.innerHeight - box.height - gap));
      panel.style.left = `${left}px`;
      panel.style.top = `${top}px`;
    },
    hide() {
      const panel = this.$root.querySelector('[popover]');
      if (panel?.matches(':popover-open')) panel.hidePopover();
    },
  }));

  // ── Header rows (components.HeaderRows) ───────────────────────────────────
  // Adds a row copied from the blank template, and removes the row whose ✕ was clicked.
  // "header-rows:clear" removes every row; "header-rows:ensure" adds one if there is
  // none. Changes are announced with an input event, as if the user had typed.
  Alpine.data('headerRows', () => ({
    init() {
      this.$root.addEventListener('header-rows:clear', () => {
        this.list().replaceChildren();
        this.changed();
      });
      this.$root.addEventListener('header-rows:ensure', () => {
        if (!this.list().querySelector('[data-header-row]')) this.append(false);
      });
    },
    list() {
      return this.$root.querySelector('[data-header-rows]');
    },
    append(focus) {
      const row = this.$root.querySelector('[data-header-row-template]').content.firstElementChild.cloneNode(true);
      this.list().append(row);
      if (focus) row.querySelector('input')?.focus();
      this.changed();
    },
    add() {
      this.append(true);
    },
    // $el is the ✕ button that was clicked. $root is looked up from $el, so it is read
    // before the row (and the button with it) leaves the document.
    remove() {
      const root = this.$root;
      this.$el.closest('[data-header-row]').remove();
      this.changed(root);
    },
    changed(root = this.$root) {
      root.dispatchEvent(new Event('input', { bubbles: true }));
    },
  }));

  // ── Code textarea (components.CodeTextarea, spec §10.4) ───────────────────
  // Tab inserts two spaces (Shift+Tab still moves the focus back); "Format JSON"
  // pretty-prints the content or says why it is not JSON. The language comes from
  // data-language and changes with a "code-editor:language" event.
  Alpine.data('codeEditor', () => ({
    language: 'Plain text',
    formatError: '',
    init() {
      this.language = this.$root.dataset.language || 'Plain text';
      this.$root.addEventListener('code-editor:language', (e) => {
        this.language = e.detail.language;
        this.formatError = '';
      });
    },
    get notJSON() {
      return this.language !== 'JSON';
    },
    textarea() {
      return this.$root.querySelector('textarea');
    },
    indent(e) {
      if (e.shiftKey || e.altKey || e.ctrlKey || e.metaKey) return;
      e.preventDefault();
      const ta = e.target;
      ta.setRangeText('  ', ta.selectionStart, ta.selectionEnd, 'end');
      ta.dispatchEvent(new Event('input', { bubbles: true }));
    },
    format() {
      const ta = this.textarea();
      try {
        ta.value = JSON.stringify(JSON.parse(ta.value), null, 2);
        this.formatError = '';
        ta.dispatchEvent(new Event('input', { bubbles: true }));
      } catch (err) {
        this.formatError = `Not valid JSON: ${err.message}`;
      }
    },
  }));

  // ── Scenario editor (pages.ScenarioForm, spec §10.3 SC3–SC4) ──────────────
  // On the editor's <form>. Keeps the body's language hint in step with the
  // Content-Type header row (the same rule as pages.BodyLanguage), clears the header
  // rows when their switch goes off, and asks before closing with unsaved changes:
  // on Close (the modal's Close button calls this component's close(), which is the
  // nearest one) and on Escape. A form re-rendered after a failed save carries
  // data-unsaved, since what it shows is not what is stored.
  const UNSAVED_QUESTION = 'You will lose unsaved changes. Are you sure you want to close?';
  Alpine.data('scenarioEditor', () => ({
    snapshot: '',
    unsaved: false,
    dialog: null,
    onCancel: null,
    init() {
      this.snapshot = this.serialize();
      this.unsaved = this.$root.hasAttribute('data-unsaved');
      this.dialog = this.$root.closest('dialog');
      this.onCancel = (e) => {
        if (this.dirty()) {
          e.preventDefault();
          this.askToClose();
        }
      };
      this.dialog?.addEventListener('cancel', this.onCancel);
    },
    destroy() {
      this.dialog?.removeEventListener('cancel', this.onCancel);
    },
    serialize() {
      return new URLSearchParams(new FormData(this.$root)).toString();
    },
    dirty() {
      return this.unsaved || this.serialize() !== this.snapshot;
    },
    update() {
      const code = this.$root.querySelector('[data-language]');
      const language = this.bodyLanguage();
      if (code && code.dataset.language !== language) {
        code.dataset.language = language;
        code.dispatchEvent(new CustomEvent('code-editor:language', { detail: { language } }));
      }
    },
    bodyLanguage() {
      for (const row of this.$root.querySelectorAll('[data-header-rows] [data-header-row]')) {
        const [name, value] = row.querySelectorAll('input');
        if (name.value.trim().toLowerCase() !== 'content-type') continue;
        const v = value.value.toLowerCase();
        if (v.includes('html')) return 'HTML';
        if (v.includes('json')) return 'JSON';
        return 'Plain text';
      }
      return 'Plain text';
    },
    changed(e) {
      if (e.target.name !== 'responseHeadersEnabled') return;
      const rows = this.$root.querySelector('[data-header-rows]')?.parentElement;
      rows?.dispatchEvent(new CustomEvent(e.target.checked ? 'header-rows:ensure' : 'header-rows:clear'));
    },
    close() {
      if (this.dirty()) {
        this.askToClose();
        return;
      }
      this.dialog?.close();
    },
    askToClose() {
      const dialog = this.dialog;
      document.dispatchEvent(new CustomEvent('confirm:ask', {
        detail: { question: UNSAVED_QUESTION, accept: () => dialog?.close() },
      }));
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
