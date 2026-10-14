import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';

export const SettingsPageContext = createContext<{
  active: boolean;
  footer: HTMLElement | null;
  reportDirty: (dirty: boolean) => void;
} | null>(null);

export function SettingsFooter({ children }: { children: ReactNode }) {
  const page = useContext(SettingsPageContext);
  if (!page) return <>{children}</>;
  return page.active && page.footer ? createPortal(children, page.footer) : null;
}

// Keep drafts in the mounted settings workspace, never in persistent browser storage.
export function useSettingsDraft<T>(value: T, restore: (value: T) => void) {
  const page = useContext(SettingsPageContext);
  const [baseline, setBaseline] = useState<T | null>(null);
  const dirty = baseline !== null && JSON.stringify(value) !== JSON.stringify(baseline);
  const reportDirty = page?.reportDirty;
  useEffect(() => { reportDirty?.(dirty); }, [dirty, reportDirty]);
  return {
    dirty,
    accept: (next: T) => setBaseline(structuredClone(next)),
    reset: () => { if (baseline !== null) restore(structuredClone(baseline)); },
  };
}
