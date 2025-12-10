export interface WidgetConfig {
  title: string;
  size: 'small' | 'medium' | 'large';
  refreshInterval?: number;
}

export abstract class WidgetBase {
  protected _container: HTMLDivElement;
  protected _config: WidgetConfig;
  protected _isLoading: boolean = false;

  constructor(container: HTMLDivElement, config: WidgetConfig) {
    this._container = container;
    this._config = config;
  }

  abstract render(): Promise<void>;
  abstract getData(): Promise<any>;
  
  protected createWidgetFrame(): HTMLDivElement {
    const widget = document.createElement('div');
    widget.classList.add('overview-widget', `widget-${this._config.size}`);
    
    const header = document.createElement('div');
    header.classList.add('widget-header');
    
    const title = document.createElement('h6');
    title.classList.add('widget-title');
    title.textContent = this._config.title;
    
    header.appendChild(title);
    widget.appendChild(header);
    
    const content = document.createElement('div');
    content.classList.add('widget-content');
    widget.appendChild(content);
    
    this._container.appendChild(widget);
    return content;
  }

  protected showLoading(content: HTMLDivElement): void {
    this._isLoading = true;
    content.innerHTML = `
      <div class="widget-loading">
        <div class="spinner-border text-primary" role="status">
          <span class="sr-only">Loading...</span>
        </div>
      </div>
    `;
  }

  protected hideLoading(): void {
    this._isLoading = false;
  }

  protected showError(content: HTMLDivElement, message: string): void {
    content.innerHTML = `
      <div class="widget-error alert alert-warning">
        <i class="fas fa-exclamation-triangle"></i>
        ${message}
      </div>
    `;
  }

  public resize(): void {
    // Override in subclasses if needed
  }

  public destroy(): void {
    if (this._container.parentNode) {
      this._container.parentNode.removeChild(this._container);
    }
  }
}