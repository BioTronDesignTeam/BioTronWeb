import { Component, type ErrorInfo, type ReactNode } from 'react';

interface SceneBoundaryProps {
  /** What to show instead of the 3D scene once it has failed. */
  fallback: ReactNode;
  children: ReactNode;
}

/**
 * Catches a 3D scene that fails, such as a lighting file or a model that does
 * not load, or a WebGL context that is lost at start-up. React Three Fiber
 * rethrows those into the page, and without a boundary the whole page goes
 * blank. With one, the visitor gets the fallback and the rest of the page.
 */
export default class SceneBoundary extends Component<SceneBoundaryProps, { failed: boolean }> {
  override state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  override componentDidCatch(error: Error, info: ErrorInfo) {
    console.warn('The 3D scene failed; showing its fallback instead.', error, info.componentStack);
  }

  override render() {
    return this.state.failed ? this.props.fallback : this.props.children;
  }
}
