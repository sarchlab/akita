export function setupTraceButton(traceBtn: HTMLButtonElement) {
    if (!traceBtn) {
        console.warn("Trace toggle button not found.");
        return;
    }

    if (traceBtn.dataset.traceInitialized === "true") {
        return;
    }
    traceBtn.dataset.traceInitialized = "true";

    const getTextNode = (): Text | null => {
        for (let i = traceBtn.childNodes.length - 1; i >= 0; i--) {
            const node = traceBtn.childNodes.item(i);
            if (node && node.nodeType === Node.TEXT_NODE) {
                return node as Text;
            }
        }
        return null;
    };

    const updateTraceBtn = (tracing: boolean) => {
        // Find the rotating icon button and img in the sibling button within the same btn-group
        const btnGroup = traceBtn.parentElement;
        const iconBtn = btnGroup?.querySelector("button:has(img)") as HTMLButtonElement | null;
        const img = btnGroup?.querySelector("img") as HTMLImageElement | null;
        const textNode = getTextNode();

        if (tracing) {
            // Tracing is running: green icon button, red stop button
            traceBtn.classList.add("btn-danger");
            traceBtn.classList.remove("btn-success");
            if (iconBtn) {
                iconBtn.classList.add("btn-success");
                iconBtn.classList.remove("btn-danger");
            }
            if (img) {
                img.classList.add("rotating-icon");
            }
            if (textNode) {
                textNode.textContent = "Stop Tracing";
            }
        } else {
            // Tracing is stopped: red icon button, green start button
            traceBtn.classList.add("btn-success");
            traceBtn.classList.remove("btn-danger");
            if (iconBtn) {
                iconBtn.classList.add("btn-danger");
                iconBtn.classList.remove("btn-success");
            }
            if (img) {
                img.classList.remove("rotating-icon");
            }
            if (textNode) {
                textNode.textContent = "Start Tracing";
            }
        }
    };

    (window as any).testTraceButton = () => {
        console.log("Testing trace button...");
        traceBtn.click();
        console.log("Button clicked! Check the UI changes.");
    };

    const attachOnlineHandler = (initialState: boolean) => {
        let tracing = initialState;
        updateTraceBtn(tracing);

        const handleClick = async () => {
            try {
                if (tracing) {
                    await fetch("/api/trace/end", { method: "POST" });
                    tracing = false;
                } else {
                    await fetch("/api/trace/start", { method: "POST" });
                    tracing = true;
                }
                updateTraceBtn(tracing);
            } catch (error) {
                console.error("Error toggling trace:", error);
            }
        };

        traceBtn.addEventListener("click", handleClick);
    };

    const attachOfflineHandler = () => {
        let tracing = false;
        updateTraceBtn(tracing);

        const handleClick = () => {
            tracing = !tracing;
            updateTraceBtn(tracing);
            console.log("Offline mode: Toggled tracing state to", tracing);
        };

        traceBtn.addEventListener("click", handleClick);
    };

    fetch("/api/trace/is_tracing")
        .then((response) => {
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            return response.json();
        })
        .then((data) => {
            attachOnlineHandler(Boolean(data.isTracing));
        })
        .catch((error) => {
            console.error("Error fetching trace status:", error);
            attachOfflineHandler();
        });
}
