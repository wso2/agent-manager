import type { ComponentPropsWithoutRef } from "react";

export type IconName =
  | "compose"
  | "search"
  | "library"
  | "sparkles"
  | "grid"
  | "chevron-down"
  | "plus"
  | "sliders"
  | "mic"
  | "wave"
  | "star"
  | "help"
  | "sun"
  | "moon";

type IconProps = ComponentPropsWithoutRef<"svg"> & {
  name: IconName;
};

const paths: Record<IconName, React.ReactNode> = {
  compose: (
    <>
      <path d="M4 15.5V20h4.5L19 9.5 14.5 5 4 15.5z" />
      <path d="M13 6.5L17.5 11" />
    </>
  ),
  search: (
    <>
      <circle cx="11" cy="11" r="6" />
      <path d="M20 20l-3.5-3.5" />
    </>
  ),
  library: (
    <>
      <path d="M4 5h12a2 2 0 0 1 2 2v11" />
      <path d="M4 5v11a2 2 0 0 0 2 2h12" />
      <path d="M8 5v13" />
    </>
  ),
  sparkles: (
    <>
      <path d="M12 3l1.8 4.5L18 9l-4.2 1.5L12 15l-1.8-4.5L6 9l4.2-1.5L12 3z" />
      <path d="M19 4l.7 1.7L21 6l-1.3.5L19 8l-.7-1.5L17 6l1.3-.3L19 4z" />
    </>
  ),
  grid: (
    <>
      <rect x="4" y="4" width="6" height="6" rx="1" />
      <rect x="14" y="4" width="6" height="6" rx="1" />
      <rect x="4" y="14" width="6" height="6" rx="1" />
      <rect x="14" y="14" width="6" height="6" rx="1" />
    </>
  ),
  "chevron-down": <path d="M6 9l6 6 6-6" />,
  plus: <path d="M12 5v14M5 12h14" />,
  sliders: (
    <>
      <path d="M4 6h10" />
      <path d="M18 6h2" />
      <circle cx="16" cy="6" r="2" />
      <path d="M4 12h4" />
      <path d="M12 12h8" />
      <circle cx="10" cy="12" r="2" />
      <path d="M4 18h12" />
      <path d="M20 18h0.01" />
      <circle cx="18" cy="18" r="2" />
    </>
  ),
  mic: (
    <>
      <rect x="9" y="3" width="6" height="11" rx="3" />
      <path d="M5 11a7 7 0 0 0 14 0" />
      <path d="M12 18v3" />
      <path d="M8 21h8" />
    </>
  ),
  wave: (
    <>
      <path d="M6 9v6" />
      <path d="M10 6v12" />
      <path d="M14 9v6" />
      <path d="M18 7v10" />
    </>
  ),
  star: (
    <path d="M12 3.5l2.6 5.3 5.8.8-4.2 4.1 1 5.8L12 16.7 6.8 19.5l1-5.8L3.6 9.6l5.8-.8L12 3.5z" />
  ),
  help: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M9.5 9.5a2.5 2.5 0 0 1 4.6 1.4c0 1.6-2.1 2.1-2.1 3.6" />
      <circle cx="12" cy="17" r="0.7" />
    </>
  ),
  sun: (
    <>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v3" />
      <path d="M12 19v3" />
      <path d="M4.22 4.22l2.12 2.12" />
      <path d="M17.66 17.66l2.12 2.12" />
      <path d="M2 12h3" />
      <path d="M19 12h3" />
      <path d="M4.22 19.78l2.12-2.12" />
      <path d="M17.66 6.34l2.12-2.12" />
    </>
  ),
  moon: (
    <path d="M21 14.5A8.5 8.5 0 0 1 9.5 3a7 7 0 1 0 11.5 11.5z" />
  ),
};

export default function Icon({ name, ...props }: IconProps) {
  return (
    <svg
      aria-hidden="true"
      focusable="false"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      {paths[name]}
    </svg>
  );
}
