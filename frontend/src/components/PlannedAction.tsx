import React from 'react';

interface PlannedActionProps {
  /** Unique within the page — ties the control to its explanation. */
  id: string;
  label: string;
  /** Why it does nothing yet. Shown, not just announced. */
  note: string;
  icon?: React.ReactNode;
  className?: string;
}

/**
 * A control the design calls for but nothing is wired to yet.
 *
 * Rendering it as an ordinary button is a lie: it looks pressable, and the
 * reader only discovers otherwise by pressing it. Rendering it `disabled`
 * removes it from the tab order, so a keyboard or screen-reader user never
 * learns the feature exists at all. `aria-disabled` is the middle road — the
 * control stays reachable and announces itself as unavailable, and the reason
 * sits beside it in plain text for everyone.
 */
const PlannedAction: React.FC<PlannedActionProps> = ({ id, label, note, icon, className = '' }) => (
  <span className="inline-flex flex-col items-start gap-1">
    <button
      type="button"
      aria-disabled="true"
      aria-describedby={`${id}-note`}
      onClick={(e) => e.preventDefault()}
      className={`btn cursor-not-allowed border border-dashed border-neutral-500 text-neutral-700 ${className}`}
    >
      {icon}
      {label}
    </button>
    <span id={`${id}-note`} className="text-xs text-neutral-700">
      {note}
    </span>
  </span>
);

export default PlannedAction;
