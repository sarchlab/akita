import { useSegments } from "../hooks/useSegments";
import type { Segment } from "../types/task";
import { smartString } from "../utils/smartValue";

interface SegmentSelectorProps {
  /** Currently selected segment index (-1 for "all"). */
  selected: number;
  /** Callback when the user picks a segment. */
  onSelect: (index: number, segment: Segment | null) => void;
}

/**
 * Dropdown selector that lists traced time segments fetched from /api/segments.
 */
export default function SegmentSelector({
  selected,
  onSelect,
}: SegmentSelectorProps) {
  const { data, loading, error } = useSegments();

  if (loading) {
    return (
      <select className="form-select form-select-sm" disabled>
        <option>Loading segments…</option>
      </select>
    );
  }

  if (error) {
    return (
      <select className="form-select form-select-sm" disabled>
        <option>Error loading segments</option>
      </select>
    );
  }

  const segments = data?.segments ?? [];

  if (!data?.enabled || segments.length === 0) {
    return (
      <select className="form-select form-select-sm" disabled>
        <option>No segments available</option>
      </select>
    );
  }

  return (
    <select
      className="form-select form-select-sm"
      value={selected}
      onChange={(e) => {
        const idx = Number(e.target.value);
        onSelect(idx, idx >= 0 ? segments[idx] : null);
      }}
    >
      <option value={-1}>All segments</option>
      {segments.map((seg, i) => (
        <option key={i} value={i}>
          Segment {i + 1}: {smartString(seg.start_time)} → {smartString(seg.end_time)}
        </option>
      ))}
    </select>
  );
}
