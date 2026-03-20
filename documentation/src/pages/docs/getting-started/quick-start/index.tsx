import { Redirect } from '@docusaurus/router';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';

export default function QuickStartRedirect(): JSX.Element {
  const { siteConfig } = useDocusaurusContext();
  const latestVersion = siteConfig.customFields?.latestVersion as string;
  return <Redirect to={`/agent-manager/docs/${latestVersion}/getting-started/quick-start/`} />;
}
