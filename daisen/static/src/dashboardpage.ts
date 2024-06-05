import Dashboard from "./dashboard";

class DashboardPage {
  _container: HTMLElement;
  _dashboard: Dashboard;

  constructor() {
    this._container = null;
    this._dashboard = new Dashboard();
  }

  layout() {
    const [_, containerHeight] = this._containerDim();
    this._container = document.getElementById("inner-container");
    this._container.style.height = (containerHeight - 60).toString() + "px";

    const dashboardCanvas = this._getDashboard();
    const pageContainer = this._getPageContainer(dashboardCanvas);
    const pageBtnContainer = this._getPageButtonContainer(dashboardCanvas);
    const toolBar = this._getToolbar();
    toolBar.innerHTML = "";

    this._dashboard.setCanvas(pageContainer, pageBtnContainer, toolBar);
    // this._dashboard.resize();
    
  }

  _containerDim(): [number, number] {
    const container = document.getElementById("container");
    const topNav = document.getElementById("top-nav");
    return [container.offsetWidth, window.innerHeight - topNav.offsetHeight];
  }

  _getDashboard(): HTMLDivElement {
    const list = this._container.getElementsByClassName("dashboard");
    let dashboardCanvas: HTMLDivElement;

    if (list.length > 0) {
      dashboardCanvas = <HTMLDivElement>list[0];
    } else {
      dashboardCanvas = document.createElement("div");
      dashboardCanvas.classList.add("dashboard");
      dashboardCanvas.classList.add("container-fluid");
      this._container.append(dashboardCanvas);
    }

    const [containerWidth, containerHeight] = this._containerDim();
    dashboardCanvas.style.width = containerWidth.toString() + "px";
    dashboardCanvas.style.height = containerHeight.toString() + "px";

    return dashboardCanvas;
  }

  _getPageContainer(dashboardCanvas: HTMLDivElement): HTMLDivElement {
    const list = dashboardCanvas.getElementsByClassName("page-container");

    let pageContainer: HTMLDivElement;

    if (list.length > 0) {
      pageContainer = <HTMLDivElement>list[0];
    } else {
      pageContainer = document.createElement("div");
      pageContainer.classList.add("row");
      pageContainer.classList.add("page-container");
      dashboardCanvas.appendChild(pageContainer);
    }

    pageContainer.style.width =
      dashboardCanvas.offsetWidth.toString() + "px";
    pageContainer.style.height =
      (dashboardCanvas.offsetHeight - 78).toString() + "px";

    return pageContainer;
  }

  _getPageButtonContainer(dashboardCanvas: HTMLDivElement): HTMLDivElement {
    const list = dashboardCanvas.getElementsByClassName(
      "page-button-container"
    );

    if (list.length > 0) {
      return <HTMLDivElement>list[0];
    }

    const pageBtnContainer = document.createElement("div");
    pageBtnContainer.classList.add("row");
    pageBtnContainer.classList.add("page-button-container");
    pageBtnContainer.classList.add("justify-content-center");
    dashboardCanvas.appendChild(pageBtnContainer);

    return pageBtnContainer;
  }

  _getToolbar(): HTMLFormElement {
    const nav = document.getElementById("top-nav");
    const forms = nav.getElementsByTagName("form");
    let toolBar = null;
    if (forms.length == 0) {
      toolBar = this._createToolbar(nav);
    } else {
      toolBar = forms[0];
    }
    return toolBar;
  }

  _createToolbar(nav: HTMLElement): HTMLFormElement {
    const toolBar = document.createElement("form");
    toolBar.id = "tool-bar";
    toolBar.classList.add("form-inline");
    toolBar.classList.add("mb-0");
    toolBar.onsubmit = e => {
      e.preventDefault();
    };
    nav.appendChild(toolBar);
    return toolBar;
  }

  render() {
    this._dashboard.render();
  }
}

export default DashboardPage;
