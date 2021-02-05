export class UIManager {
    navBar: HTMLElement
    mainContainer: HTMLElement
    treeContainer: HTMLElement
    verticalDivider: HTMLElement
    detailContainer: HTMLElement
    treeContainerWidthPercent: number


    constructor() {
        this.treeContainerWidthPercent = 0.3
    }

    assignElements() {
        this.navBar = document.getElementById("navbar")
        this.mainContainer = document.getElementById("main-container")
        this.treeContainer = document.getElementById("tree-container")
        this.verticalDivider = document.getElementById("vertical-divider")
        this.detailContainer = document.getElementById("detail-container")
    }

    resize() {
        const windowHeight = window.innerHeight;
        const windowWidth = window.innerWidth;

        const navHeight = this.navBar.offsetHeight;
        const containerHeight = windowHeight - navHeight;

        const verticalDividerWidth = 5;
        let treeWidth = windowWidth * this.treeContainerWidthPercent;
        if (treeWidth > windowWidth - verticalDividerWidth) {
            treeWidth = windowWidth - verticalDividerWidth;
        }

        const detailContainerWidth =
            windowWidth - treeWidth - verticalDividerWidth;

        this.mainContainer.style.height = `${containerHeight}px`
        this.mainContainer.style.width = `${windowWidth}px`
        this.treeContainer.style.height = `${containerHeight}px`;
        this.treeContainer.style.width = `${treeWidth}px`;
        this.verticalDivider.style.height = `${containerHeight}px`;
        this.verticalDivider.style.width = `${verticalDividerWidth}px`;
        this.detailContainer.style.height = `${containerHeight}px`;
        this.detailContainer.style.width = `${detailContainerWidth}px`;
    }
}
