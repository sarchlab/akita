export class UIManager {
    isLeftVerticalBarDragging: Boolean = false
    isRightVerticalBarDragging: Boolean = false
    isMonitorGroupDividerDragging: Boolean = false

    navBar: HTMLElement | null = null
    mainContainer: HTMLElement | null = null
    leftPane: HTMLElement | null = null
    centralPane: HTMLElement | null = null
    rightPane: HTMLElement | null = null
    verticalDividerLeft: HTMLElement | null = null
    verticalDividerRight: HTMLElement | null = null
    progressBarGroupContainer: HTMLElement | null = null
    mainContainerHeight: number = 0
    monitorGroupHeight: number = 0
    leftPaneWidth: number
    rightPaneWidth: number
    verticalDividerWidth: number = 8;
    horizontalDividerHeight: number = 6;
    navBarHeight: number = 56

    debugButton: HTMLElement | null = null
    profileButton: HTMLElement | null = null
    mode = 'debug'


    constructor() {
        this.leftPaneWidth = 200
        this.rightPaneWidth = 700
    }

    assignElements() {
        this.navBar = document.getElementById("navbar")
        this.mainContainer = document.getElementById("debug-tool-container")
        this.leftPane = document.getElementById("left-pane")
        this.centralPane = document.getElementById("central-pane")
        this.rightPane = document.getElementById("right-pane")

        this.verticalDividerLeft =
            document.getElementById("vertical-divider-left")!
        this.verticalDividerRight =
            document.getElementById("vertical-divider-right")!

        this.progressBarGroupContainer =
            document.getElementById("progress-bar-group")

        this.verticalDividerLeft.addEventListener(
            'mousedown',
            this.leftVerticalDividerMouseDownHandler.bind(this),
        )

        this.verticalDividerRight.addEventListener(
            'mousedown',
            this.rightVerticalDividerMouseDownHandler.bind(this),
        )

        document.addEventListener(
            'mousemove',
            (e) => {
                this.verticalDividerMouseMoveHandler(e)
                this.horizontalDividerMouseMoveHandler(e)
            }
        )

        document.addEventListener(
            'mouseup',
            (e) => {
                this.verticalDividerMouseUpHandler(e)
                this.horizontalDividerMouseUpHandler(e)
            }

        )

        document.getElementById("monitor-group-divider")!.addEventListener(
            'mousedown',
            this.monitorGroupDividerMouseDownHandler.bind(this),
        )

        // this.debugButton = document.getElementById("debug-button")
        // this.profileButton = document.getElementById("profile-button")

        // this.debugButton.addEventListener('click', () => {
        //     this.switchMode('debug')
        // })
        // this.profileButton.addEventListener('click', () => {
        //     this.switchMode('profile')
        // })

        this.mainContainerHeight = window.innerHeight -
            this.navBarHeight -
            this.horizontalDividerHeight -
            this.progressBarGroupContainer!.offsetHeight

        // this.switchMode('debug')
        this.resize()
    }

    leftVerticalDividerMouseDownHandler(_: MouseEvent) {
        this.isLeftVerticalBarDragging = true;
    }

    rightVerticalDividerMouseDownHandler(_: MouseEvent) {
        this.isRightVerticalBarDragging = true;
    }

    monitorGroupDividerMouseDownHandler(_: MouseEvent) {
        this.isMonitorGroupDividerDragging = true;
    }

    verticalDividerMouseMoveHandler(e: MouseEvent) {
        if (this.isLeftVerticalBarDragging) {
            this.leftPaneWidth = e.clientX - this.verticalDividerWidth / 2
            this.resize();
        }

        if (this.isRightVerticalBarDragging) {
            this.rightPaneWidth = window.innerWidth -
                e.clientX -
                this.verticalDividerWidth / 2
            this.resize();
        }
    }

    horizontalDividerMouseMoveHandler(e: MouseEvent) {
        if (this.isMonitorGroupDividerDragging) {
            this.mainContainerHeight =
                e.clientY - this.navBarHeight -
                this.horizontalDividerHeight / 2

            this.monitorGroupHeight =
                window.innerHeight -
                this.mainContainerHeight -
                this.navBarHeight - this.horizontalDividerHeight -
                this.progressBarGroupContainer!.offsetHeight

            const maxMainContainerHeight = window.innerHeight -
                this.navBarHeight -
                this.horizontalDividerHeight -
                this.progressBarGroupContainer!.offsetHeight
            if (this.mainContainerHeight > maxMainContainerHeight) {
                this.mainContainerHeight = maxMainContainerHeight
            }

            this.resize();
        }
    }

    verticalDividerMouseUpHandler(_: MouseEvent) {
        this.isLeftVerticalBarDragging = false;
        this.isRightVerticalBarDragging = false
    }

    horizontalDividerMouseUpHandler(_: MouseEvent) {
        this.isMonitorGroupDividerDragging = false;
    }

    adjustProgressBarGroupHeight() {
        this.mainContainerHeight = window.innerHeight -
            this.navBarHeight -
            this.horizontalDividerHeight -
            this.monitorGroupHeight -
            this.progressBarGroupContainer!.offsetHeight
        this.resize();
    }

    popUpMonitorGroup() {
        this.monitorGroupHeight = 200
        this.mainContainerHeight = window.innerHeight -
            this.navBarHeight -
            this.horizontalDividerHeight -
            this.monitorGroupHeight -
            this.progressBarGroupContainer!.offsetHeight
        this.resize();
    }

    collapseMonitorGroup() {
        this.monitorGroupHeight = 0
        this.mainContainerHeight = window.innerHeight -
            this.navBarHeight -
            this.horizontalDividerHeight -
            this.progressBarGroupContainer!.offsetHeight
        this.resize();
    }

    resize() {
        const windowWidth = window.innerWidth;

        const detailContainerWidth = windowWidth -
            this.leftPaneWidth - this.rightPaneWidth -
            this.verticalDividerWidth * 2;

        this.mainContainer!.style.height = `${this.mainContainerHeight}px`
        this.mainContainer!.style.width = `${windowWidth}px`
        this.leftPane!.style.height = `${this.mainContainerHeight}px`;
        this.leftPane!.style.width = `${this.leftPaneWidth}px`;
        this.centralPane!.style.height = `${this.mainContainerHeight}px`;
        this.centralPane!.style.width = `${detailContainerWidth}px`;
        this.rightPane!.style.height = `${this.mainContainerHeight}px`;
        this.rightPane!.style.width = `${this.rightPaneWidth}px`;

        this.verticalDividerLeft!.style.height =
            `${this.mainContainerHeight}px`;
        this.verticalDividerLeft!.style.width =
            `${this.verticalDividerWidth}px`;
        this.verticalDividerRight!.style.height =
            `${this.mainContainerHeight}px`;
        this.verticalDividerRight!.style.width =
            `${this.verticalDividerWidth}px`;

        this.resizeMonitorGroup()
    }

    private resizeMonitorGroup() {
        const container = document.getElementById("monitor-group-container")!

        container.style.height = `${this.monitorGroupHeight}px`

        const widgets = container.querySelectorAll(".monitor-widget")

        const widgetMargin = 10
        const windowWidth = window.innerWidth
        const marginCount = widgets.length + 1
        const widgetWidth = (windowWidth - widgetMargin * marginCount) / widgets.length

        for (let i = 0; i < widgets.length; i++) {
            const widget = widgets[i] as HTMLElement
            widget.style.width = `${widgetWidth}px`
            widget.style.height = `${this.monitorGroupHeight - 10}px`

            const chart: HTMLElement = widget.querySelector('.chart')!
            const widgetTitle: HTMLElement = widget.querySelector('.title')!
            const widgetTitleHeight = widgetTitle.offsetHeight
            chart.style.height =
                `${this.monitorGroupHeight - 18 - widgetTitleHeight}px`
        }
    }

    // private switchMode(mode: string) {
    //     this.mode = mode
    //     this.setModeSwitchingButtonStyle()
    //     this.resize()
    // }

    // setModeSwitchingButtonStyle() {
    //     switch (this.mode) {
    //         case 'debug':
    //             this.debugButton.classList.remove('btn-outline-primary')
    //             this.debugButton.classList.add('btn-primary')
    //             this.profileButton.classList.remove('btn-primary')
    //             this.profileButton.classList.add('btn-outline-primary')
    //             break
    //         case 'profile':
    //             this.debugButton.classList.remove('btn-primary')
    //             this.debugButton.classList.add('btn-outline-primary')
    //             this.profileButton.classList.remove('btn-outline-primary')
    //             this.profileButton.classList.add('btn-primary')
    //             break
    //     }
    // }
}