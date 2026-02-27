import type { FC, ComponentType } from "react";

declare module "@agent-management-platform/llm-providers" {
  export const metaData: {
    title: string;
    description: string;
    icon: ComponentType<{ size?: number }>;
    path: string;
    component: FC;
    levels: {
      component: FC;
      organization: FC;
    };
  };
}

