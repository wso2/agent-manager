import { AuthProvider } from "@agent-management-platform/auth";
import { ClientProvider } from "@agent-management-platform/api-client";
import { OxygenUIThemeProvider } from "@wso2/oxygen-ui";

export const GlobalProviders = ({ children }: { children: React.ReactNode }) => {
  return (
      <OxygenUIThemeProvider  radialBackground>
        <AuthProvider>
          <ClientProvider>
            {children}
          </ClientProvider>
        </AuthProvider>
        </OxygenUIThemeProvider>
  );
};
