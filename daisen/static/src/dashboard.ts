import * as d3 from "d3";
import Widget from "./widget";
import { thresholdFreedmanDiaconis } from "d3";

class YAxisOption {
  optionValue: string;
  html: string;
}

class Dashboard {
  _canvas: HTMLDivElement;
  _pageBtnContainer: HTMLDivElement;
  _toolBar: HTMLFormElement;

  _componentNames: Array<string>;
  _filteredNames: Array<string>;
  _numWidget: number;
  _numRow: number;
  _numCol: number;
  _currPage: number;
  _currFilter: string;
  _filterTimer: NodeJS.Timeout;
  _primaryAxis: string;
  _secondaryAxis: string;
  _startTime: number;
  _endTime: number;
  _widgets: Array<Widget>;
  _yAxisOptions: Array<YAxisOption>;
  _initialWidth: number;
  _initialHeight: number;

  constructor() {
    this._numWidget = 16;
    this._numRow = 4;
    this._numCol = 4;
    this._widgets = [];
    this._yAxisOptions = [
      { optionValue: "ReqInCount", html: "Incoming Request Rate" },
      { optionValue: "ReqCompleteCount", html: "Request Complete Rate" },
      { optionValue: "AvgLatency", html: "Average Request Latency" },
      { optionValue: "ConcurrentTask", html: "Number Concurrent Task" },
      { optionValue: "BufferPressure", html: "Buffer Pressure" },
      { optionValue: "PendingReqOut", html: "Pending Request Out" },
      { optionValue: "-", html: " - " },
    ];
    this._initialWidth = window.innerWidth;
    this._initialHeight = window.innerHeight;
  }

  setCanvas(
    canvas: HTMLDivElement,
    pageBtnContainer: HTMLDivElement,
    toolBar: HTMLFormElement
  ) {
    this._canvas = canvas;
    this._pageBtnContainer = pageBtnContainer;
    this._toolBar = toolBar;
  
    this._canvas.classList.add('canvas-container');
    
    const burgerMenu = document.createElement('div');
    burgerMenu.classList.add('burger-menu');
    burgerMenu.innerHTML = `
      <div class="burger-bar"></div>
      <div class="burger-bar"></div>
      <div class="burger-bar"></div>
    `;
    burgerMenu.style.position = 'absolute';
    burgerMenu.style.top = '10px';
    burgerMenu.style.right = '10px';

    const dropdownCanvas = document.createElement('div');
    dropdownCanvas.classList.add('dropdown-canvas');
    dropdownCanvas.style.display = 'none';

    document.body.appendChild(burgerMenu);
    document.body.appendChild(dropdownCanvas);
  
    burgerMenu.addEventListener('click', () => {
      const isActive = dropdownCanvas.classList.toggle('active');
      dropdownCanvas.style.display = isActive ? 'block' : 'none';
    });
  
    window.addEventListener('resize', () => {
      this._updateNavbarVisibility(); 
      this._resize();
      this._widgets.forEach((w: Widget) => {
        w.createWidget(this._widgetWidth(), this._widgetHeight());
        w.setXAxis(this._startTime, this._endTime);
        w.setFirstAxis(this._primaryAxis);
        w.setSecondAxis(this._secondaryAxis);
      });
      const paginationContainer = this._canvas.querySelector('.pagination-container') as HTMLElement;
      if (paginationContainer) {
        paginationContainer.style.left = '50%';
        paginationContainer.style.transform = 'translateX(-50%)';
      }
      this.render();
    }); 

    this._addZoomResetButton(this._toolBar);
    this._addFilterUI(this._toolBar);
    this._addPrimarySelector(this._toolBar);
    this._addSecondarySelector(this._toolBar);
    this._addZoomResetButton(dropdownCanvas);
    this._addFilterUI(dropdownCanvas);
    this._addPrimarySelector(dropdownCanvas);
    this._addSecondarySelector(dropdownCanvas);
    this._resize();

  }

  _updateNavbarVisibility() {
    if (window.innerWidth <= 1365) {
      this._toolBar.style.display = 'none';
      const burgerMenu = document.querySelector('.burger-menu') as HTMLElement;
      if (burgerMenu) burgerMenu.style.display = 'block';
    } else {
      this._toolBar.style.display = 'flex';
      const burgerMenu = document.querySelector('.burger-menu') as HTMLElement;
      if (burgerMenu) burgerMenu.style.display = 'none';
      const dropdownCanvas = document.querySelector('.dropdown-canvas') as HTMLElement;
      if (dropdownCanvas) {
        dropdownCanvas.classList.remove('active');
        dropdownCanvas.style.display = 'none';
      }
    }
  }

  _resize() {
    this._resetNumRowCol();
    const width = this._widgetWidth();
    const height = this._widgetHeight();
    this._widgets.forEach((w: Widget) => {
      w.resize(width, height);
    });
  }

  _resetNumRowCol() {
    const rowColTable = [
      [0, 0],
      [1, 1],
      [2, 1],
      [2, 2],
      [2, 2],
      [2, 3],
      [2, 3],
      [3, 3],
      [3, 3],
      [3, 3],
      [3, 4],
      [3, 4],
      [3, 4],
      [4, 4],
      [4, 4],
      [4, 4],
      [4, 4],
    ];
    const width = window.innerWidth;
    const height = window.innerHeight;
    this._numCol = rowColTable[this._numWidget][0];
    this._numRow = rowColTable[this._numWidget][1];
    if (width >= 1200) {
      this._numCol = 4;
    }
    if (width < 1200 && width >= 800) {
      this._numCol = 3;
    }
    if (width < 800) {
      this._numCol = 2;
    }
    console.log(width, height);
  }

  _widgetWidth(): number {
    this._resetNumRowCol();
    const numGap = this._numCol + 1;
    const marginLeft = 5;
    const gapSpace = numGap * marginLeft;
    const widgetSpace = this._canvas.offsetWidth - gapSpace;
    const width = Math.floor(widgetSpace / this._numCol);
    return width;
  }

  _widgetHeight(): number {
    this._resetNumRowCol();
    const numGap = this._numRow + 1;
    const marginTop = 5;
    const gapSpace = numGap * marginTop;
    const widgetSpace = this._canvas.offsetHeight - gapSpace;
    const height = Math.floor(widgetSpace / this._numRow);
    return height;
  }

  async _resetZoom() {
    let startTime = 0;
    let endTime = 0;
    await fetch("/api/trace?kind=Simulation")
      .then((rsp) => rsp.json())
      .then((rsp) => {
        if (rsp[0]) {
          startTime = rsp[0].start_time;
          endTime = rsp[0].end_time;
        } else {
          startTime = 0;
          endTime = 0.000001;
        }
      });
    this.setTimeRange(startTime, endTime);
  }

  setTimeRange(startTime: number, endTime: number) {
    this._startTime = startTime;
    this._endTime = endTime;

    const params = new URLSearchParams(window.location.search);
    params.set("starttime", startTime.toString());
    params.set("endtime", endTime.toString());
    window.history.replaceState(null, null, `/dashboard?${params.toString()}`);

    this._widgets.forEach((w) => {
      w.temporaryTimeShift(startTime, endTime);
      w.render(false);
    });
  }

  _addZoomResetButton(container: HTMLElement) {
    const btn = document.createElement('button');
    btn.setAttribute('type', 'button');
    btn.classList.add('btn', 'btn-primary', 'mr-3');
    btn.id = "dashboard-btn";
    btn.innerHTML = 'Reset Zoom';
    btn.onclick = () => {
      this._resetZoom();
    };
    container.appendChild(btn);
  }
  
  _addFilterUI(container: HTMLElement) {
    const filterGroup = document.createElement('div');
    filterGroup.classList.add('input-group', 'mr-3');
    filterGroup.id = 'dashboard-filter-group';
    filterGroup.innerHTML = `
      <div class="input-group-prepend">
        <span class="input-group-text" id="basic-addon1">Filter</span>
      </div>
      <input id="dashboard-filter-input" type="text" class="form-control" 
        placeholder="Component Name" aria-label="Filter" aria-describedby="basic-addon1">
    `;
    container.appendChild(filterGroup);
  
    const input = filterGroup.getElementsByTagName('input')[0];
    input.oninput = () => {
      this._filterInputChage();
    };
  }
  
  _addPrimarySelector(container: HTMLElement) {
    const selectorGroup = document.createElement('div');
    selectorGroup.classList.add('input-group', 'ml-3');
    const button = document.createElement('div');
    button.classList.add('input-group-prepend');
    selectorGroup.id = 'primary-selector-group'; 
    button.innerHTML = `
      <label class="input-group-text" for="primary-axis-select">
        <div class="circle-primary"></div>
        <div class="circle-primary"></div>
        <div class="circle-primary"></div>
        Primary Y-Axis
      </label>`;
    selectorGroup.appendChild(button);
  
    const select = document.createElement('select');
    select.classList.add('custom-select');
    select.id = 'primary-axis-select';
    selectorGroup.appendChild(select);
  
    this._yAxisOptions.forEach((o, index) => {
      let option = document.createElement('option');
      if (index === 0) {
        option.selected = true;
      }
      option.value = o.optionValue;
      option.innerHTML = o.html;
      select.add(option);
    });
  
    select.onchange = () => {
      this._switchPrimaryAxis(select.value);
    };
  
    container.appendChild(selectorGroup);
    this._canvas.classList.add('canvas-container');
  }
  
  _addSecondarySelector(container: HTMLElement) {
    const selectorGroup = document.createElement('div');
    selectorGroup.classList.add('input-group', 'ml-3');
    const button = document.createElement('div');
    button.classList.add('input-group-prepend');
    selectorGroup.id = 'secondary-selector-group';
    button.innerHTML = `
      <label class="input-group-text" for="secondary-axis-select">
        <div class="circle-secondary"></div>
        <div class="circle-secondary"></div>
        <div class="circle-secondary"></div>
        Secondary Y-Axis
      </label>`;
    selectorGroup.appendChild(button);
  
    const select = document.createElement('select');
    select.classList.add('custom-select');
    select.id = 'secondary-axis-select';
    selectorGroup.appendChild(select);
  
    this._yAxisOptions.forEach((o, index) => {
      let option = document.createElement('option');
      if (index === 0) {
        option.selected = true;
      }
      option.value = o.optionValue;
      option.innerHTML = o.html;
      select.add(option);
    });
  
    select.onchange = () => {
      this._switchSecondaryAxis(select.value);
    };
  
    container.appendChild(selectorGroup);
  }
  

  _switchPrimaryAxis(name: string) {
    this._primaryAxis = name;
    this._widgets.forEach((w) => {
      const params = new URLSearchParams(window.location.search);
      params.set("primary_y", name);
      window.history.pushState(null, null, `/dashboard?${params.toString()}`);

      w.setFirstAxis(name);
      w.render(true);
    });
  }

  _switchSecondaryAxis(name: string) {
    this._secondaryAxis = name;
    this._widgets.forEach((w) => {
      const params = new URLSearchParams(window.location.search);
      params.set("second_y", name);
      window.history.pushState(null, null, `/dashboard?${params.toString()}`);

      w.setSecondAxis(name);
      w.render(true);
    });
  }

  _filterInputChage() {
    window.clearTimeout(this._filterTimer);
    this._filterTimer = setTimeout(() => {
      const filterInput = <HTMLInputElement>(
        document.getElementById("dashboard-filter-input")
      );
      const filterString = filterInput.value;
      this._currPage = 0;

      const params = new URLSearchParams(window.location.search);
      params.set("filter", filterString);
      params.set("page", "0");
      window.history.pushState(null, null, `/dashboard?${params.toString()}`);

      this._currFilter = filterString;
      this._filter();
    }, 1000);
  }

  render() {
    Promise.all([fetch("/api/trace?kind=Simulation"), fetch("/api/compnames")])
      .then(([simulation, compNames]) => {
        return Promise.all([simulation.json(), compNames.json()]);
      })
      .then(([simulation, compNames]) => {
        simulation = simulation[0];

        compNames.sort();

        this._componentNames = compNames;
        this._filteredNames = compNames;

        this._getParamsFromURL(simulation);
        this._updateNavbarVisibility();
        this._filter();
      });
  }

  _getParamsFromURL(simulation: Object) {
    const params = new URLSearchParams(window.location.search);
    let page = params.get("page");
    let pageNum = 0;
    if (page === null) {
      pageNum = 0;
    } else {
      pageNum = parseInt(page);
    }

    let filter = params.get("filter");
    if (filter === null) {
      filter = "";
    }
    const dashboardFilter = <HTMLInputElement>(
      document.getElementById("dashboard-filter-input")
    );
    dashboardFilter.value = filter;

    let primaryAxisName = params.get("primary_y");
    if (primaryAxisName === null) {
      primaryAxisName = "ReqInCount";
    }
    const primaryAxisSelect = <HTMLInputElement>(
      document.getElementById("primary-axis-select")
    );
    primaryAxisSelect.value = primaryAxisName;

    let secondaryAxisName = params.get("second_y");
    if (secondaryAxisName === null) {
      secondaryAxisName = "AvgLatency";
    }
    const secondaryAxisSelect = <HTMLInputElement>(
      document.getElementById("secondary-axis-select")
    );
    secondaryAxisSelect.value = secondaryAxisName;

    let startTime = params.get("starttime");
    let endTime = params.get("endtime");
    if (startTime !== null && endTime !== null) {
      this.setTimeRange(parseFloat(startTime), parseFloat(endTime));
    } else if (!simulation) {
      this.setTimeRange(0, 0.000001);
    } else {
      this.setTimeRange(simulation["start_time"], simulation["end_time"]);
    }

    this._currPage = pageNum;
    this._currFilter = filter;
    this._primaryAxis = primaryAxisName;
    this._secondaryAxis = secondaryAxisName;
  }

  _filter() {
    if (this._currFilter === "") {
      this._filteredNames = this._componentNames;
    } else {
      const re = new RegExp(this._currFilter);
      this._filteredNames = this._componentNames.filter(
        (v) => v.search(re) >= 0
      );
    }

    this._numWidget = this._filteredNames.length;
    if (this._numWidget > 16) {
      this._numWidget = 16;
    }
    this._resetNumRowCol();

    this._addPaginationControl();
    this._renderPage();
  }

  _addPaginationControl() {
    const nav = document.createElement("nav");
    nav.classList.add("mt-4");
    const ul = document.createElement("ul");
    ul.classList.add("pagination");
    nav.appendChild(ul);

    const pageInfo = document.createElement("div");
    pageInfo.classList.add("page-info");
  
    const paginationContainer = document.createElement("div");
    paginationContainer.classList.add("pagination-container");
    paginationContainer.appendChild(nav);
    paginationContainer.appendChild(pageInfo);
    if (this._canvas.querySelector('.pagination-container')) {
      this._canvas.removeChild(this._canvas.querySelector('.pagination-container'));
    }
    this._canvas.appendChild(paginationContainer);
    this._addPageButtons(ul);
  }

  _addPageButtons(ul: HTMLUListElement) {
    const numPages = Math.ceil(
      this._filteredNames.length / (this._numRow * this._numCol)
    );

    if (numPages === 0) {
      this._showNoComponentInfo();
      return;
    }

    this._addPrevPageButton(ul, this._currPage);

    this._addNumPageButton(ul, 0);

    if (this._currPage > 2) {
      this._addEllipsis(ul);
    }

    for (let i = Math.max(1, this._currPage - 1); i <= Math.min(numPages - 2, this._currPage + 1); i++) {
      this._addNumPageButton(ul, i);
    }

    if (this._currPage < numPages - 3) {
      this._addEllipsis(ul);
    }

    if (numPages > 1) {
      this._addNumPageButton(ul, numPages - 1);
    }

    this._addNextPageButton(ul, this._currPage, numPages);
    // let offset = -2;
    // if (this._currPage <= 1) {
    //   offset = -this._currPage;
    // }
    // if (this._currPage == numPages - 2) {
    //   offset = -3;
    // }
    // if (this._currPage == numPages - 1) {
    //   offset = -4;
    // }

    // for (let i = 0; i < 5; i++) {
    //   const pageNum = this._currPage + i + offset;
    //   if (pageNum < 0 || pageNum >= numPages) {
    //     continue;
    //   }

    //   this._addNumPageButton(ul, pageNum);
    // }

    // this._addNextPageButton(ul, this._currPage, numPages);
  }

  _addEllipsis(ul: HTMLUListElement) {
    const li = document.createElement("li");
    li.classList.add("page-item", "disabled");
    li.innerHTML = `<a class="page-link">...</a>`;
    ul.appendChild(li);
  }

  _showNoComponentInfo() {
    const div = document.createElement("div");
    div.classList.add("alert");
    div.classList.add("alert-warning");
    div.innerHTML = "No component selected.";

    this._pageBtnContainer.innerHTML = "";
    this._pageBtnContainer.appendChild(div);
  }

  _addPrevPageButton(ul: HTMLUListElement, currPageNum: number) {
    const li = document.createElement("li");
    li.classList.add("page-item");
    if (currPageNum == 0) {
      li.classList.add("disabled");
    }
    li.innerHTML = `
            <a class="page-link" aria-label="Previous">
                <span aria-hidden="true">&laquo;</span>
                <span class="sr-only">Previous</span>
            </a>
        `;

    if (currPageNum > 0) {
      li.onclick = () => {
        this._switchToPage(currPageNum - 1);
      };
    }

    ul.appendChild(li);
  }

  _addNextPageButton(
    ul: HTMLUListElement,
    currPageNum: number,
    totalNumPages: number
  ) {
    const li = document.createElement("li");
    li.classList.add("page-item");
    if (currPageNum == totalNumPages - 1) {
      li.classList.add("disabled");
    }
    li.innerHTML = `
            <a class="page-link" aria-label="Next">
                <span aria-hidden="true">&raquo;</span>
                <span class="sr-only">Next</span
            </a>
        `;

    if (currPageNum < totalNumPages - 1) {
      li.onclick = () => {
        this._switchToPage(currPageNum + 1);
      };
    }

    ul.appendChild(li);
  }

  _addNumPageButton(ul: HTMLUListElement, pageNum: number) {
    const li = document.createElement("li");
    li.classList.add("page-item");
    if (pageNum == this._currPage) {
      li.classList.add("active");
    }
    li.innerHTML = `
            <a class="page-link">
                ${pageNum + 1}
            </a>
        `;

    li.onclick = () => {
      this._switchToPage(pageNum);
    };

    ul.appendChild(li);
  }

  _switchToPage(pageNum: number) {
    this._currPage = pageNum;

    const params = new URLSearchParams(window.location.search);
    params.set("page", pageNum.toString());
    window.history.pushState(null, null, `/dashboard?${params.toString()}`);

    this._addPaginationControl();
    this._renderPage();
    const pageInfo = this._pageBtnContainer.querySelector('.page-info') as HTMLDivElement;
  }

  _renderPage() {
    this._canvas.innerHTML = "";
    const startIndex = this._currPage * this._numRow * this._numCol;
    const endIndex = startIndex + this._numRow * this._numCol;
    const componentsToShow = this._filteredNames.slice(startIndex, endIndex);

    this._widgets = [];
    componentsToShow.forEach((name) => {
      const widget = new Widget(name, this._canvas, this);
      this._widgets.push(widget);

      widget.createWidget(this._widgetWidth(), this._widgetHeight());

      widget.setXAxis(this._startTime, this._endTime);
      widget.setFirstAxis(this._primaryAxis);
      widget.setSecondAxis(this._secondaryAxis);

      widget.render(true);
    });
    this._addPaginationControl();
  }
}

export default Dashboard;