/** A labelled checkbox in Akita's style. */
export default function Toggle({ checked, onChange, children, title }: {
  checked: boolean; onChange: (v: boolean) => void; children: React.ReactNode; title?: string;
}) {
  return (
    <label className="inline-flex cursor-pointer items-center gap-2 whitespace-nowrap text-xs font-medium text-muted-foreground" title={title}>
      <input type="checkbox" className="h-4 w-4 accent-primary" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      {children}
    </label>
  );
}
