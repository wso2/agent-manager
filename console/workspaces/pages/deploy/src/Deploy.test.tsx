import { render, screen } from '@testing-library/react';
import { 
  DeployComponent,
  DeployProject,
  DeployOrganization,
} from './index';

describe('DeployComponent', () => {
  it('renders without crashing', () => {
    render(<DeployComponent />);
    expect(screen.getByText('Deploy - Component Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Title';
    render(<DeployComponent title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Description';
    render(<DeployComponent description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays component level indicator', () => {
    render(<DeployComponent />);
    expect(screen.getByText('Component Level View')).toBeInTheDocument();
  });
});

describe('DeployProject', () => {
  it('renders without crashing', () => {
    render(<DeployProject />);
    expect(screen.getByText('Deploy - Project Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Project Title';
    render(<DeployProject title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Project Description';
    render(<DeployProject description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays project level indicator', () => {
    render(<DeployProject />);
    expect(screen.getByText('Project Level View')).toBeInTheDocument();
  });
});

describe('DeployOrganization', () => {
  it('renders without crashing', () => {
    render(<DeployOrganization />);
    expect(screen.getByText('Deploy - Organization Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Organization Title';
    render(<DeployOrganization title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Organization Description';
    render(<DeployOrganization description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays organization level indicator', () => {
    render(<DeployOrganization />);
    expect(screen.getByText('Organization Level View')).toBeInTheDocument();
  });
});
