import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { getLocale } from "./lib/i18n";
import "./styles.css";

// Dark mode follows the system setting (Akita defines only light variables).
function followScheme() {
  try {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => document.documentElement.classList.toggle("dark", mq.matches);
    apply();
    mq.addEventListener("change", apply);
  } catch { /* ignore */ }
}
followScheme();
document.documentElement.lang = getLocale() === "zh" ? "zh-CN" : "en";

const root = document.getElementById("root");
if (!root) {
  throw new Error("Root element not found");
}

createRoot(root).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
