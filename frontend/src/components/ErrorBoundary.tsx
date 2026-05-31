import { Component, ErrorInfo, ReactNode } from 'react';
import './ErrorBoundary.css';

interface Props {
    children: ReactNode;
    fallback?: ReactNode;
}

interface State {
    hasError: boolean;
    error: Error | null;
    errorInfo: ErrorInfo | null;
}

class ErrorBoundary extends Component<Props, State> {
    public state: State = {
        hasError: false,
        error: null,
        errorInfo: null
    };

    public static getDerivedStateFromError(error: Error): Partial<State> {
        return { hasError: true, error };
    }

    public componentDidCatch(error: Error, errorInfo: ErrorInfo) {
        console.error('Uncaught error:', error, errorInfo);
        this.setState({ errorInfo });
    }

    private handleReset = () => {
        this.setState({ hasError: false, error: null, errorInfo: null });
    };

    public render() {
        if (this.state.hasError) {
            if (this.props.fallback) {
                return this.props.fallback;
            }

            return (
                <div className="error-boundary">
                    <div className="error-boundary-icon">⚠️</div>
                    <h1 className="error-boundary-title">Что-то пошло не так</h1>
                    <p className="error-boundary-message">
                        Произошла непредвиденная ошибка. Пожалуйста, попробуйте перезагрузить приложение.
                    </p>
                    {import.meta.env.DEV && (
                        <details className="error-boundary-details">
                            <summary>Показать детали ошибки</summary>
                            <div className="error-boundary-trace">
                                {this.state.error && this.state.error.toString()}
                                <br />
                                {this.state.errorInfo?.componentStack}
                            </div>
                        </details>
                    )}
                    <button className="error-boundary-button" onClick={this.handleReset}>
                        Попробовать снова
                    </button>
                </div>
            );
        }

        return this.props.children;
    }
}

export default ErrorBoundary;
