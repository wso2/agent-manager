import type {ReactNode} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';

import styles from './index.module.css';

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={clsx('hero', styles.heroBanner)}>
      <div className="container">
        <Heading as="h1" className="hero__title">
          {siteConfig.title}
        </Heading>
        <p className="hero__subtitle">{siteConfig.tagline}</p>
        <div className={styles.buttons}>
          <Link
            className="button button--primary button--lg"
            to="/docs/intro">
            Get Started →
          </Link>
          <Link
            className="button button--secondary button--lg margin-left--md"
            to="https://github.com/wso2/ai-agent-management-platform">
            View on GitHub
          </Link>
        </div>
      </div>
    </header>
  );
}

function FeatureSection() {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="row">
          <div className="col col--4">
            <div className="text--center padding-horiz--md">
              <Heading as="h3">🚀 Deploy at Scale</Heading>
              <p>
                Deploy and run AI agents on Kubernetes with production-ready configurations,
                auto-scaling, and high availability.
              </p>
            </div>
          </div>
          <div className="col col--4">
            <div className="text--center padding-horiz--md">
              <Heading as="h3">🔍 Full Observability</Heading>
              <p>
                Capture traces, metrics, and logs for complete visibility into agent behavior
                using OpenTelemetry instrumentation.
              </p>
            </div>
          </div>
          <div className="col col--4">
            <div className="text--center padding-horiz--md">
              <Heading as="h3">🛡️ Governance</Heading>
              <p>
                Enforce policies, manage access controls, and ensure compliance across all
                agents with built-in governance tools.
              </p>
            </div>
          </div>
        </div>
        <div className="row margin-top--lg">
          <div className="col col--4">
            <div className="text--center padding-horiz--md">
              <Heading as="h3">🔧 Auto-Instrumentation</Heading>
              <p>
                Zero-code instrumentation for popular AI frameworks including LangChain,
                LlamaIndex, and more.
              </p>
            </div>
          </div>
          <div className="col col--4">
            <div className="text--center padding-horiz--md">
              <Heading as="h3">📊 Lifecycle Management</Heading>
              <p>
                Manage agent versions, configurations, and deployments from a unified control
                plane with rollback capabilities.
              </p>
            </div>
          </div>
          <div className="col col--4">
            <div className="text--center padding-horiz--md">
              <Heading as="h3">🌐 External Agent Support</Heading>
              <p>
                Monitor and govern externally deployed agents alongside internal ones with
                unified observability.
              </p>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

function QuickStartSection() {
  return (
    <section className={styles.quickStart}>
      <div className="container">
        <div className="row">
          <div className="col col--8 col--offset-2">
            <Heading as="h2" className="text--center margin-bottom--lg">
              Get Started in Minutes
            </Heading>
            <p className="text--center margin-bottom--lg">
              Try the platform with our quick-start dev container. Everything you need is pre-configured.
            </p>
            <div className={styles.codeBlock}>
              <pre>
                <code>
{`docker run --rm -it --name amp-quick-start \\
  -v /var/run/docker.sock:/var/run/docker.sock \\
  --network=host \\
  ghcr.io/wso2/amp-quick-start:v0.0.0-dev

# Inside container
./install.sh`}
                </code>
              </pre>
            </div>
            <div className="text--center margin-top--lg">
              <Link
                className="button button--primary button--lg"
                to="/docs/getting-started/quick-start">
                View Full Quick Start Guide →
              </Link>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

export default function Home(): ReactNode {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
      title={`Home`}
      description="Deploy, manage, and govern AI agents at scale with WSO2 AI Agent Management Platform">
      <HomepageHeader />
      <main>
        <FeatureSection />
        <QuickStartSection />
      </main>
    </Layout>
  );
}
