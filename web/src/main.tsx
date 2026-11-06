import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { App } from "./App";
import { LiveProvider } from "./state/LiveProvider";
import { ThemeProvider } from "./state/ThemeProvider";
import "./styles.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <ThemeProvider>
        <LiveProvider>
          <App />
        </LiveProvider>
      </ThemeProvider>
    </BrowserRouter>
  </StrictMode>,
);
