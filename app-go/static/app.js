// tmock UI script (spec §10.1).
//
// Loaded with `defer` after htmx and hx-alpine-compat, and before Alpine, so the
// components below are registered before Alpine starts. Alpine is the CSP build:
// components are registered here with Alpine.data(), never written inline in HTML.
//
// htmx configuration is NOT set here: htmx 4 reads <meta name="htmx-config"> while its
// script loads, which is too early for this file. See internal/views/layout.

document.addEventListener('alpine:init', () => {
  // Shows on the page that both libraries are running (I00 verification).
  Alpine.data('jsStatus', () => ({
    message: `htmx ${htmx.version} and Alpine ${Alpine.version} are running.`,
  }));
});

document.addEventListener('alpine:initialized', () => {
  console.info(`tmock: htmx ${htmx.version}, Alpine ${Alpine.version}`);
});
