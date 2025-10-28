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
        const img = traceBtn.querySelector("img") as HTMLImageElement | null;
        const textNode = getTextNode();

        if (img) {
            img.classList.add("loading");
        }

        if (tracing) {
            traceBtn.classList.add("btn-danger");
            traceBtn.classList.remove("btn-success");
            if (img) {
                img.src = "rotate_icon.png";
                img.alt = "Rotate Icon";
                img.className = "rotating-icon";
            }
            if (textNode) {
                textNode.textContent = " Stop";
            }
        } else {
            traceBtn.classList.add("btn-success");
            traceBtn.classList.remove("btn-danger");
            if (img) {
                img.src = "rotate_icon.png";
                img.alt = "Stop Icon";
                img.className = "";
            }
            if (textNode) {
                textNode.textContent = " Start";
            }
        }

        if (img) {
            img.onload = () => img.classList.remove("loading");
            img.onerror = () => img.classList.remove("loading");
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
