import { useEffect, useState } from "react";
import type { ChangeEvent } from "react";
import { useGitHubStatus } from "../../hooks/useGitHubStatus";

interface GitHubAttachmentProps {
  disabled?: boolean;
  onChange: (selectedKeys: string[]) => void;
}

export default function GitHubAttachment({ disabled = false, onChange }: GitHubAttachmentProps) {
  const { available, routineKeys, loading } = useGitHubStatus();
  const [enabled, setEnabled] = useState(false);
  const [selectedKeys, setSelectedKeys] = useState<string[]>([]);

  useEffect(() => {
    setSelectedKeys((prev) => prev.filter((key) => routineKeys.includes(key)));
  }, [routineKeys]);

  useEffect(() => {
    onChange(enabled ? selectedKeys : []);
  }, [enabled, onChange, selectedKeys]);

  if (loading) {
    return <small className="text-muted">Checking GitHub routine availability…</small>;
  }

  if (!available) {
    return null;
  }

  const handleToggleKey = (event: ChangeEvent<HTMLInputElement>, key: string) => {
    const checked = event.target.checked;
    setSelectedKeys((prev) => {
      if (checked) {
        return prev.includes(key) ? prev : [...prev, key];
      }

      return prev.filter((item) => item !== key);
    });
  };

  return (
    <div className="border rounded p-2 bg-light-subtle">
      <div className="form-check mb-0">
        <input
          id="github-attachment-toggle"
          checked={enabled}
          className="form-check-input"
          disabled={disabled}
          onChange={(event) => setEnabled(event.target.checked)}
          type="checkbox"
        />
        <label className="form-check-label small fw-semibold" htmlFor="github-attachment-toggle">
          Attach GitHub routines
        </label>
      </div>

      {enabled && (
        <div className="mt-2 d-flex flex-column gap-2">
          <div className="d-flex align-items-center justify-content-between">
            <small className="text-muted">
              Selected ({selectedKeys.length}/{routineKeys.length})
            </small>
            <div className="d-flex gap-1">
              <button
                className="btn btn-outline-primary btn-sm py-0 px-2"
                disabled={disabled || routineKeys.length === 0}
                onClick={() => setSelectedKeys(routineKeys)}
                type="button"
              >
                All
              </button>
              <button
                className="btn btn-outline-secondary btn-sm py-0 px-2"
                disabled={disabled || selectedKeys.length === 0}
                onClick={() => setSelectedKeys([])}
                type="button"
              >
                None
              </button>
            </div>
          </div>

          <div className="border rounded p-2 bg-white" style={{ maxHeight: "130px", overflowY: "auto" }}>
            {routineKeys.map((key, index) => {
              const inputId = `github-routine-${index}`;

              return (
                <div className="form-check" key={key}>
                  <input
                    checked={selectedKeys.includes(key)}
                    className="form-check-input"
                    disabled={disabled}
                    id={inputId}
                    onChange={(event) => handleToggleKey(event, key)}
                    type="checkbox"
                  />
                  <label className="form-check-label small" htmlFor={inputId}>
                    {key}
                  </label>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
