// mermaid-init.js — lazy-loads mermaid.min.js only when mermaid code blocks
// are present. Keeps script-src 'self' CSP compliant (no inline JS).
(function () {
  "use strict";

  var blocks = document.querySelectorAll("pre code.language-mermaid");
  if (blocks.length === 0) return;

  // Convert <pre><code class="language-mermaid"> to <div class="mermaid">
  // before mermaid.js loads so it can find them.
  blocks.forEach(function (el) {
    var pre = el.parentElement;
    var div = document.createElement("div");
    div.className = "mermaid";
    div.textContent = el.textContent;
    pre.parentElement.replaceChild(div, pre);
  });

  var darkQuery = window.matchMedia("(prefers-color-scheme: dark)");

  function mermaidTheme() {
    return darkQuery.matches ? "dark" : "default";
  }

  var script = document.createElement("script");
  script.src = "/static/js/mermaid.min.js";
  script.onload = function () {
    mermaid.initialize({
      startOnLoad: false,
      theme: mermaidTheme(),
      securityLevel: "strict",
    });
    mermaid.run();

    // Re-render when system colour scheme changes.
    darkQuery.addEventListener("change", function () {
      mermaid.initialize({
        startOnLoad: false,
        theme: mermaidTheme(),
        securityLevel: "strict",
      });
      mermaid.run();
    });
  };
  document.body.appendChild(script);
})();
