(function () {
  // --- Study page: reveal answer and rate with keyboard -------------------
  const show = document.getElementById("show");
  const answer = document.getElementById("answer");
  const ratings = document.getElementById("ratings");

  function reveal() {
    if (!answer || !answer.hidden) return;
    answer.hidden = false;
    ratings.hidden = false;
    show.hidden = true;
  }
  if (show) show.addEventListener("click", reveal);

  document.addEventListener("keydown", (e) => {
    if (!show || e.target.matches("input, textarea, select")) return;
    if (e.key === " " || e.key === "Enter") {
      if (answer.hidden) { e.preventDefault(); reveal(); }
    } else if (!answer.hidden && ["1", "2", "3", "4"].includes(e.key)) {
      ratings.querySelector(`button[value="${e.key}"]`).click();
    }
  });

  // --- German text-to-speech ---------------------------------------------
  document.querySelectorAll(".speak").forEach((btn) => {
    if (!("speechSynthesis" in window)) { btn.hidden = true; return; }
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      speechSynthesis.cancel();
      const u = new SpeechSynthesisUtterance(btn.dataset.say);
      u.lang = "de-DE";
      const voice = speechSynthesis.getVoices().find((v) => v.lang.startsWith("de"));
      if (voice) u.voice = voice;
      speechSynthesis.speak(u);
    });
  });

  // --- Umlaut buttons insert into the last focused field --------------------
  let lastField = document.querySelector("input[name=front], textarea[name=text]");
  document.addEventListener("focusin", (e) => {
    if (e.target.matches("input:not([type=hidden]):not([type=checkbox]), textarea")) lastField = e.target;
  });
  document.querySelectorAll(".umlauts button").forEach((b) => {
    b.addEventListener("mousedown", (e) => e.preventDefault()); // keep focus
    b.addEventListener("click", () => {
      if (!lastField) return;
      const s = lastField.selectionStart ?? lastField.value.length;
      const t = lastField.selectionEnd ?? s;
      lastField.setRangeText(b.dataset.char, s, t, "end");
      lastField.focus();
    });
  });

  // --- Installable app (needs HTTPS or localhost) ---------------------------
  if ("serviceWorker" in navigator) navigator.serviceWorker.register("/sw.js");

  // --- Confirm before delete -------------------------------------------------
  document.querySelectorAll("form[data-confirm]").forEach((f) => {
    f.addEventListener("submit", (e) => {
      if (!confirm(f.dataset.confirm)) e.preventDefault();
    });
  });
})();
