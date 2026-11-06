import { Component, type ErrorInfo, type ReactNode } from "react";

interface Props {
  children: ReactNode;
}
interface State {
  error: Error | null;
}

// ErrorBoundary prevents a single render error from blanking the whole app
// (the "black screen"). It shows the error and a reload button instead.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Log for debugging; the UI still renders a readable fallback.
    console.error("UI render error:", error, info);
  }

  render() {
    if (this.state.error) {
      return (
        <div className="panel panel-body">
          <h2 style={{ margin: 0, fontSize: 16 }}>Es ist ein Anzeige-Fehler aufgetreten</h2>
          <p className="muted">
            Die Oberfläche hat einen Fehler abgefangen, statt abzustürzen.
          </p>
          <pre
            style={{
              whiteSpace: "pre-wrap",
              color: "var(--err)",
              fontSize: 12,
            }}
          >
            {this.state.error.message}
          </pre>
          <button
            className="btn"
            onClick={() => {
              this.setState({ error: null });
            }}
          >
            Erneut versuchen
          </button>{" "}
          <button className="btn secondary" onClick={() => location.reload()}>
            Seite neu laden
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
