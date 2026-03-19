import { Redirect } from '@docusaurus/router';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';

export default function DocsRedirect(): JSX.Element {
  const { siteConfig } = useDocusaurusContext();
  const latestVersion = siteConfig.customFields?.latestVersion as string;
  return <Redirect to={`${latestVersion}/overview/what-is-amp/`} />;
}
