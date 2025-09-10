import DashboardPage from "./dashboardpage";
import { TaskPage } from "./taskpage";
import { MouseEventHandler } from "./mouseeventhandler";

interface View {
  layout(): void;
}

class App {
  _view: string;
  _dashboardPage: DashboardPage;
  _taskPage: TaskPage;
  _currentView: View;

  constructor() {
    this._view = "landing_page";

    this._dashboardPage = new DashboardPage();
    this._taskPage = new TaskPage();

    this._currentView = null;
  }

  start() {
    window.addEventListener(
      "popstate",
      () => {
        this.route();
      },
      false
    );

    window.addEventListener("resize", () => {
      this._currentView.layout();
    });

    this.route();
  }

  route() {
    const path = window.location.pathname;

    if (path === "/" || path === "/dashboard") {
      this.switchLayout(this._dashboardPage);
      this._dashboardPage.render();
      return;
    }

    if (path === "/task") {
      this._showTaskPage();
    }

    if (path === "/component") {
      this._showComponentPage();
    }
  }

  _showTaskPage() {
    this.switchLayout(this._taskPage);
    const mouseEventHanlder = new MouseEventHandler(this._taskPage);
    mouseEventHanlder.register(this._taskPage);

    let search = window.location.search;
    const params = new URLSearchParams(search);
    const taskID = params.get("id");
    if (taskID === null) {
      fetch("/api/trace?kind=Simulation")
        .then(rsp => rsp.json())
        .then(rsp => {
          window.history.pushState(null, null, `/task?id=${rsp[0].id}`);
          return rsp;
        })
        .then(rsp => {
          this._taskPage.showTask(rsp[0]);
        });
      return;
    } else {
      fetch(`/api/trace?id=${taskID}`)
        .then(rsp => rsp.json())
        .then(rsp => {
          if (!params.has("starttime") || !params.has("endtime")) {
            this._taskPage.showTask(rsp[0]);
            return;
          }

          const startTime = parseFloat(params.get("starttime"));
          const endTime = parseFloat(params.get("endtime"));
          this._taskPage.setTimeRange(startTime, endTime, false);
          this._taskPage.showTask(rsp[0], true);
        });
      return;
    }
  }

  async _showComponentPage() {
    this.switchLayout(this._taskPage);
    const mouseEventHanlder = new MouseEventHandler(this._taskPage);
    mouseEventHanlder.register(this._taskPage);

    let search = window.location.search;
    const params = new URLSearchParams(search);
    const componentName = params.get("name");
    let startTimeArg = params.get("starttime");
    let endTimeArg = params.get("endtime");
    let startTime = parseFloat(startTimeArg);
    let endTime = parseFloat(endTimeArg);

    if (startTimeArg === null || endTimeArg === null) {
      await fetch("/api/trace?kind=Simulation")
        .then(rsp => rsp.json())
        .then(rsp => {
          startTime = rsp[0].start_time;
          endTime = rsp[0].end_time;
          params.set("starttime", startTime.toString());
          params.set("endTime", endTime.toString());
          window.history.replaceState(
            null,
            null,
            `/component?${params.toString()}`
          );
        });
      this._taskPage.setTimeRange(startTime, endTime, false);
    } else {
      this._taskPage.setTimeRange(startTime, endTime, false);
    }

    this._taskPage.showComponent(componentName);

    return;
  }

  switchLayout(layout: View) {
    if (this._currentView == layout) {
      return;
    }

    this._currentView = layout;
    this._currentView.layout();
  }
}

export default App;
