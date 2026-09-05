import React from "react";
import ReactDOM from "react-dom/client";
import { initI18n } from "./i18n";
import { AppRuntimeGuard } from "./components/AppRuntimeGuard";
import {
  captureError,
  initErrorReporter,
  markFrontendReady,
  recordFrontendStage,
} from "./utils/errorReporter";
import { setBootSplashStage, showBootSplashError } from "./utils/bootSplash";
import { hydrateUiPreferences } from "./utils/uiPreferences";

initErrorReporter();
recordFrontendStage("script_loaded");
setBootSplashStage("script_loaded");

void Promise.all([initI18n(), hydrateUiPreferences()]).then(async () => {
  const { default: App } = await import("./App");

  const rootElement = document.getElementById("root");
  if (!rootElement) {
    const error = new Error("Root element not found");
    captureError(error, { source: "frontend_boot", phase: "root_lookup" });
    throw error;
  }

  recordFrontendStage("react_mount_start");
  ReactDOM.createRoot(rootElement).render(
    <React.StrictMode>
      <AppRuntimeGuard>
        <App />
      </AppRuntimeGuard>
    </React.StrictMode>,
  );

  window.requestAnimationFrame(() => {
    setBootSplashStage("react_mounted");
    markFrontendReady("react_mounted");
  });
}).catch((error: unknown) => {
  captureError(error, { source: "frontend_boot", phase: "startup" });
  showBootSplashError(error instanceof Error ? error.message : String(error));
});
