import { AccountControls, Brand } from '@biotron/style';
import MachineSelector from './MachineSelector';
import ModeToggle, { type ViewMode } from './ModeToggle';
import { PRODUCT_NAME } from '../config';

interface TopBarProps {
  machines: string[];
  selectedMachine: string;
  onSelectMachine: (id: string) => void;
  mode: ViewMode;
  onModeChange: (mode: ViewMode) => void;
}

/**
 * One row from `lg` up: brand and machine on the left, the mode switch in the
 * middle, the account controls on the right. Narrower, three equal columns
 * no longer fit, so the switch drops to a second row of its own; on a phone
 * the wordmark also gives way to the hand mark so the machine select keeps
 * its room.
 */
export default function TopBar({
  machines,
  selectedMachine,
  onSelectMachine,
  mode,
  onModeChange,
}: TopBarProps) {
  return (
    <header className="sticky top-0 z-30 border-b border-[#aedbfc] bg-white/90 backdrop-blur dark:border-white/[0.14] dark:bg-surface/90">
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2 px-4 py-2.5 sm:px-6 sm:py-3 lg:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
        {/* Left: brand and machine selector */}
        <div className="flex min-w-0 items-center gap-3 justify-self-start">
          <span className="shrink-0 sm:hidden">
            <Brand compact />
          </span>
          {/* The wordmark at the width every other BioTron header uses. */}
          <span className="hidden w-28 shrink-0 sm:block">
            <Brand className="w-28" />
          </span>
          <span
            className="hidden h-6 w-px bg-[#16033c]/15 lg:block dark:bg-white/20"
            aria-hidden="true"
          />
          <span className="hidden truncate text-sm font-semibold tracking-wide text-[#16033c] lg:block dark:text-white">
            {PRODUCT_NAME}
          </span>
          <MachineSelector
            machines={machines}
            selected={selectedMachine}
            onSelect={onSelectMachine}
          />
        </div>

        {/* Center: Live / Historical toggle, on its own row below lg */}
        <div className="col-span-2 row-start-2 justify-self-center lg:col-span-1 lg:col-start-2 lg:row-start-1">
          <ModeToggle mode={mode} onModeChange={onModeChange} />
        </div>

        {/* Right: theme toggle + account menu */}
        <AccountControls className="col-start-2 row-start-1 justify-self-end lg:col-start-3" />
      </div>
    </header>
  );
}
