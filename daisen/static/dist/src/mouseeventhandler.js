export class MouseEventHandler {
    constructor(view) {
        this._view = view;
        this._isDragging = false;
        this._dragStartMouseX = 0;
        this._dragStartStartTime = 0;
        this._dragStartEndTime = 0;
    }
    register(view) {
        this._view = view;
        const dom = view.domElement();
        dom.addEventListener("mousemove", e => {
            this.handleMouseMove(e);
        });
        dom.addEventListener("mousedown", e => {
            this.handleMouseDown(e);
        });
        dom.addEventListener("mouseup", e => {
            this.handleMouseUp(e);
        });
        dom.addEventListener("touchstart", e => {
            this.handleTouchStart(e);
        });
        dom.addEventListener("touchend", e => {
            this.handleTouchEnd(e);
        });
        dom.addEventListener("touchmove", e => {
            this.handleTouchMove(e);
        });
        dom.addEventListener("wheel", e => {
            this.handleMouseWheel(e);
        });
    }
    handleTouchStart(e) {
        // e.preventDefault();
        const touches = e.touches;
        if (touches.length == 1) {
            this.handleTouchDragStart(e);
        }
        if (touches.length == 2) {
            this.handlePinchStart([e.touches[0], e.touches[1]]);
        }
    }
    handleTouchEnd(e) {
        // e.preventDefault();
        this._isPinching = false;
        this._isDragging = false;
    }
    handleTouchMove(e) {
        // e.preventDefault();
        if (this._isDragging) {
            this.handleTouchDrag(e);
        }
        if (this._isPinching) {
            this.handlePinch(e);
        }
    }
    handleTouchDragStart(e) {
        this._isDragging = true;
        this._isPinching = false;
        const [startTime, endTime] = this._view.getAxisStatus();
        this._dragStartStartTime = startTime;
        this._dragStartEndTime = endTime;
        this._dragStartMouseX = e.touches[0].clientX;
        this._pinchStartTouches = [];
        for (let i = 0; i < e.touches.length; i++) {
            this._pinchStartTouches.push(e.touches[i]);
        }
    }
    handleTouchDrag(e) {
        const x = e.touches[0].clientX;
        const [startTime, endTime, leftX, rightX] = this._view.getAxisStatus();
        this.handleMouseDrag(startTime, endTime, leftX, rightX, x);
        this.continueScrolling();
    }
    handlePinchStart(touches) {
        this._isPinching = true;
        this._isDragging = false;
        this._pinchStartTouches = touches;
        [
            this._dragStartStartTime,
            this._dragStartEndTime
        ] = this._view.getAxisStatus();
    }
    handlePinch(e) {
        const touches = e.touches;
        const startDist = Math.abs(this._pinchStartTouches[1].clientX - this._pinchStartTouches[0].clientX);
        const endDist = Math.abs(touches[1].clientX - touches[0].clientX);
        const startDuration = this._dragStartEndTime - this._dragStartStartTime;
        const scale = startDist / endDist;
        const endDuration = startDuration * scale;
        const midTime = (this._dragStartStartTime + this._dragStartEndTime) / 2;
        const newStartTime = midTime - endDuration / 2;
        const newEndTime = midTime + endDuration / 2;
        this._view.temporaryTimeShift(newStartTime, newEndTime);
        this.continueScrolling();
    }
    handleMouseMove(e) {
        // e.preventDefault();
        const [startTime, endTime, leftX, rightX] = this._view.getAxisStatus();
        this.handleMouseDrag(startTime, endTime, leftX, rightX, e.offsetX);
    }
    handleMouseDown(e) {
        e.preventDefault();
        this._isDragging = true;
        this._dragMoved = false;
        this._dragStartMouseX = e.offsetX;
        const [startTime, endTime] = this._view.getAxisStatus();
        this._dragStartStartTime = startTime;
        this._dragStartEndTime = endTime;
    }
    handleMouseUp(e) {
        e.preventDefault();
        if (this._isDragging) {
            this._isDragging = false;
        }
        if (this._dragMoved) {
            this.triggerReload();
        }
    }
    handleMouseWheel(e) {
        e.preventDefault();
        if (e.deltaY != 0) {
            this.handleMouseWheelY(e);
            this.continueScrolling();
        }
        if (e.deltaX != 0) {
            this.handleMouseWheelX(e);
            this.continueScrolling();
        }
    }
    continueScrolling() {
        window.clearTimeout(this._scrollingTimer);
        this._scrollingTimer = setTimeout(() => {
            this.triggerReload();
        }, 1000);
    }
    handleMouseWheelX(e) {
        let [startTime, endTime] = this._view.getAxisStatus();
        const duration = endTime - startTime;
        startTime += duration * (e.deltaX * 0.001);
        endTime += duration * (e.deltaX * 0.001);
        this._view.temporaryTimeShift(startTime, endTime);
    }
    handleMouseWheelY(e) {
        const [startTime, endTime, leftX, rightX] = this._view.getAxisStatus();
        const duration = endTime - startTime;
        const timePerPixel = duration / (rightX - leftX);
        const pixelOnRight = rightX - e.offsetX;
        const pixelOnLeft = e.offsetX - leftX;
        const timeOnLeft = pixelOnLeft * timePerPixel;
        const mouseTime = timeOnLeft + startTime;
        let newTimePerPixel = timePerPixel;
        if (e.deltaY > 0) {
            for (let i = 0; i < e.deltaY; i++) {
                newTimePerPixel *= 1.001;
            }
        }
        else {
            for (let i = 0; i < -e.deltaY; i++) {
                newTimePerPixel /= 1.001;
            }
        }
        const newTimeOnLeft = newTimePerPixel * pixelOnLeft;
        const newTimeOnRight = newTimePerPixel * pixelOnRight;
        const newStartTime = mouseTime - newTimeOnLeft;
        const newEndTime = mouseTime + newTimeOnRight;
        this._view.temporaryTimeShift(newStartTime, newEndTime);
    }
    handleMouseDrag(startTime, endTime, leftX, rightX, mouseX) {
        if (!this._isDragging) {
            return;
        }
        const delta = mouseX - this._dragStartMouseX;
        if (delta > 0.01 || delta < -0.01) {
            this._dragMoved = true;
        }
        const timePerPixel = (endTime - startTime) / (rightX - leftX);
        const timeDelta = timePerPixel * delta;
        const newStartTime = this._dragStartStartTime - timeDelta;
        const newEndTime = this._dragStartEndTime - timeDelta;
        this._view.temporaryTimeShift(newStartTime, newEndTime);
    }
    triggerReload() {
        const [startTime, endTime] = this._view.getAxisStatus();
        this._view.permanentTimeShift(startTime, endTime);
    }
}
