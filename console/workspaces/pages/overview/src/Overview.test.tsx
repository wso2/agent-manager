import { render, screen } from '@testing-library/react';
import { 
  OverviewComponent,
  OverviewProject,
  OverviewOrganization,
} from './index';

describe('OverviewComponent', () => {
  it('renders without crashing', () => {
    render(<OverviewComponent />);
    expect(screen.getByText('Overview - Component Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Title';
    render(<OverviewComponent title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Description';
    render(<OverviewComponent description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays component level indicator', () => {
    render(<OverviewComponent />);
    expect(screen.getByText('Component Level View')).toBeInTheDocument();
  });
});

describe('OverviewProject', () => {
  it('renders without crashing', () => {
    render(<OverviewProject />);
    expect(screen.getByText('Overview - Project Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Project Title';
    render(<OverviewProject title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Project Description';
    render(<OverviewProject description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays project level indicator', () => {
    render(<OverviewProject />);
    expect(screen.getByText('Project Level View')).toBeInTheDocument();
  });
});

describe('OverviewOrganization', () => {
  it('renders without crashing', () => {
    render(<OverviewOrganization />);
    expect(screen.getByText('Overview - Organization Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Organization Title';
    render(<OverviewOrganization title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Organization Description';
    render(<OverviewOrganization description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays organization level indicator', () => {
    render(<OverviewOrganization />);
    expect(screen.getByText('Organization Level View')).toBeInTheDocument();
  });
});
