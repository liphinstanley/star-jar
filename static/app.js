// Sounds are made with the Web Audio API, so there are no sound files to ship.
let audio;

function ctx() {
  audio ??= new AudioContext();
  return audio;
}

function tone(freq, start, dur) {
  const a = ctx();
  const o = a.createOscillator();
  const g = a.createGain();
  const t = a.currentTime + start;
  o.type = "sine";
  o.frequency.value = freq;
  g.gain.setValueAtTime(0.0001, t);
  g.gain.exponentialRampToValueAtTime(0.3, t + 0.02);
  g.gain.exponentialRampToValueAtTime(0.0001, t + dur);
  o.connect(g).connect(a.destination);
  o.start(t);
  o.stop(t + dur);
}

const sounds = {
  tap: () => tone(660, 0, 0.12),
  star: () => { tone(784, 0, 0.2); tone(1047, 0.12, 0.3); tone(1319, 0.24, 0.45); },
  spend: () => { tone(523, 0, 0.15); tone(392, 0.1, 0.25); },
};

// Browsers only allow sound after the user has tapped something.
document.addEventListener("click", (e) => {
  ctx().resume();
  if (e.target.closest("button.tile")) sounds.tap();
});

// Stars falling into the jar.
function celebrate(n) {
  const wrap = document.querySelector(".jar-wrap");
  if (!wrap) return;
  for (let i = 0; i < Math.min(n, 10); i++) {
    const s = document.createElement("span");
    s.className = "burst";
    s.textContent = "⭐";
    s.style.left = 35 + Math.random() * 30 + "%";
    s.style.animationDelay = i * 0.12 + "s";
    s.addEventListener("animationend", () => s.remove());
    wrap.appendChild(s);
  }
}

// After every page update, compare the jar with what it was before.
let lastTotal = null;

function checkTotal() {
  const app = document.querySelector("#app[data-total]");
  if (!app) return;
  const total = Number(app.dataset.total);
  if (lastTotal !== null && total > lastTotal) {
    sounds.star();
    celebrate(total - lastTotal);
  } else if (lastTotal !== null && total < lastTotal) {
    sounds.spend();
  }
  lastTotal = total;
}

document.addEventListener("DOMContentLoaded", checkTotal);
document.addEventListener("htmx:afterSettle", checkTotal);
