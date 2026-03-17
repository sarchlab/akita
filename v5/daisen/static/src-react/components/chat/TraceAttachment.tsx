import { useEffect, useMemo, useState } from "react";
import type { ChangeEvent } from "react";
import { useComponentNames } from "../../hooks/useComponentNames";
import { useTraceData } from "../../hooks/useTraceData";
import type { TraceInformation } from "../../types/chat";

interface TraceAttachmentProps {
  disabled?: boolean;
  onChange: (traceInfo: TraceInformation) => void;
}

const EMPTY_TRACE_INFO: TraceInformation = {
  selected: 0,
  startTime: 0,
  endTime: 0,
  selectedComponentNameList: [],
};

export default function TraceAttachment({ disabled = false, onChange }: TraceAttachmentProps) {
  const { names, loading: namesLoading } = useComponentNames();
  const { tasks } = useTraceData({ kind: "Simulation" });

  const componentNames = useMemo(() => [...names].sort(), [names]);
  const traceStart = tasks[0]?.start_time ?? 0;
  const traceEnd = tasks[0]?.end_time ?? 0;

  const [enabled, setEnabled] = useState(false);
  const [startTime, setStartTime] = useState<number>(traceStart);
  const [endTime, setEndTime] = useState<number>(traceEnd);
  const [selectedComponents, setSelectedComponents] = useState<string[]>([]);

  useEffect(() => {
    setStartTime(traceStart);
    setEndTime(traceEnd);
  }, [traceEnd, traceStart]);

  useEffect(() => {
    setSelectedComponents((prev) => prev.filter((name) => componentNames.includes(name)));
  }, [componentNames]);

  useEffect(() => {
    if (!enabled) {
      onChange(EMPTY_TRACE_INFO);
      return;
    }

    const minTime = Math.min(startTime, endTime);
    const maxTime = Math.max(startTime, endTime);

    onChange({
      selected: selectedComponents.length > 0 ? 1 : 0,
      startTime: minTime,
      endTime: maxTime,
      selectedComponentNameList: selectedComponents,
    });
  }, [enabled, endTime, onChange, selectedComponents, startTime]);

  const handleToggle = (event: ChangeEvent<HTMLInputElement>) => {
    const nextEnabled = event.target.checked;
    setEnabled(nextEnabled);

    if (nextEnabled && selectedComponents.length === 0 && componentNames.length > 0) {
      setSelectedComponents(componentNames);
    }
  };

  const handleComponentSelect = (event: ChangeEvent<HTMLSelectElement>) => {
    const values = Array.from(event.target.selectedOptions, (option) => option.value);
    setSelectedComponents(values);
  };

  return (
    <div className="border rounded p-2 bg-light-subtle">
      <div className="form-check mb-0">
        <input
          id="trace-attachment-toggle"
          checked={enabled}
          className="form-check-input"
          disabled={disabled}
          onChange={handleToggle}
          type="checkbox"
        />
        <label className="form-check-label small fw-semibold" htmlFor="trace-attachment-toggle">
          Attach trace context
        </label>
      </div>

      {enabled && (
        <div className="mt-2 d-flex flex-column gap-2">
          <div className="row g-2">
            <div className="col-6">
              <label className="form-label form-label-sm mb-1 small">Start time (s)</label>
              <input
                className="form-control form-control-sm"
                disabled={disabled}
                onChange={(event) => setStartTime(Number.parseFloat(event.target.value) || 0)}
                step="any"
                type="number"
                value={startTime}
              />
            </div>
            <div className="col-6">
              <label className="form-label form-label-sm mb-1 small">End time (s)</label>
              <input
                className="form-control form-control-sm"
                disabled={disabled}
                onChange={(event) => setEndTime(Number.parseFloat(event.target.value) || 0)}
                step="any"
                type="number"
                value={endTime}
              />
            </div>
          </div>

          <div className="d-flex align-items-center justify-content-between">
            <small className="text-muted">
              Components ({selectedComponents.length}/{componentNames.length})
            </small>
            <div className="d-flex gap-1">
              <button
                className="btn btn-outline-primary btn-sm py-0 px-2"
                disabled={disabled || componentNames.length === 0}
                onClick={() => setSelectedComponents(componentNames)}
                type="button"
              >
                All
              </button>
              <button
                className="btn btn-outline-secondary btn-sm py-0 px-2"
                disabled={disabled || selectedComponents.length === 0}
                onClick={() => setSelectedComponents([])}
                type="button"
              >
                None
              </button>
            </div>
          </div>

          <select
            className="form-select form-select-sm"
            disabled={disabled || namesLoading || componentNames.length === 0}
            multiple
            onChange={handleComponentSelect}
            size={Math.min(6, Math.max(componentNames.length, 2))}
            value={selectedComponents}
          >
            {componentNames.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
        </div>
      )}
    </div>
  );
}
