export class UIManager {
    isLeftVerticalBarDragging: Boolean = false
    isRightVerticalBarDragging: Boolean = false

    navBar: HTMLElement
    mainContainer: HTMLElement
    leftPane: HTMLElement
    centralPane: HTMLElement
    rightPane: HTMLElement
    verticalDividerLeft: HTMLElement
    verticalDividerRight: HTMLElement
    progressBarGroupContainer: HTMLElement
    leftPaneWidth: number
    rightPaneWidth: number
    verticalDividerWidth: number = 8;


    constructor() {
        this.leftPaneWidth = 200
        this.rightPaneWidth = 700
    }

    assignElements() {
        this.navBar = document.getElementById("navbar")
        this.mainContainer = document.getElementById("main-container")
        this.leftPane = document.getElementById("left-pane")
        this.centralPane = document.getElementById("central-pane")
        this.rightPane = document.getElementById("right-pane")

        this.verticalDividerLeft =
            document.getElementById("vertical-divider-left")
        this.verticalDividerRight =
            document.getElementById("vertical-divider-right")

        this.progressBarGroupContainer =
            document.getElementById("progress-bar-group")

        this.verticalDividerLeft.addEventListener(
            'mousedown',
            this.leftDividerMouseDownHandler.bind(this),
        )

        this.verticalDividerRight.addEventListener(
            'mousedown',
            this.rightDividerMouseDownHandler.bind(this),
        )

        this.mainContainer.addEventListener(
            'mousemove',
            this.dividerMouseMoveHandler.bind(this),
        )

        this.mainContainer.addEventListener(
            'mouseup',
            this.dividerMouseUpHandler.bind(this),
        )
    }

    leftDividerMouseDownHandler(e: MouseEvent) {
        this.isLeftVerticalBarDragging = true;
    }

    rightDividerMouseDownHandler(e: MouseEvent) {
        this.isRightVerticalBarDragging = true;
    }

    dividerMouseMoveHandler(e: MouseEvent) {
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

    dividerMouseUpHandler(e: MouseEvent) {
        this.isLeftVerticalBarDragging = false;
        this.isRightVerticalBarDragging = false
    }

    resize() {
        const windowHeight = window.innerHeight;
        const windowWidth = window.innerWidth;

        const navHeight = this.navBar.offsetHeight;
        const progressBarGroupHeight =
            this.progressBarGroupContainer.offsetHeight
        const containerHeight =
            windowHeight - navHeight - progressBarGroupHeight;

        const detailContainerWidth = windowWidth -
            this.leftPaneWidth - this.rightPaneWidth -
            this.verticalDividerWidth * 2;

        this.mainContainer.style.height = `${containerHeight}px`
        this.mainContainer.style.width = `${windowWidth}px`
        this.leftPane.style.height = `${containerHeight}px`;
        this.leftPane.style.width = `${this.leftPaneWidth}px`;
        this.centralPane.style.height = `${containerHeight}px`;
        this.centralPane.style.width = `${detailContainerWidth}px`;
        this.rightPane.style.height = `${containerHeight}px`;
        this.rightPane.style.width = `${this.rightPaneWidth}px`;

        this.verticalDividerLeft.style.height = `${containerHeight}px`;
        this.verticalDividerLeft.style.width = `${this.verticalDividerWidth}px`;
        this.verticalDividerRight.style.height = `${containerHeight}px`;
        this.verticalDividerRight.style.width = `${this.verticalDividerWidth}px`;
    }
}