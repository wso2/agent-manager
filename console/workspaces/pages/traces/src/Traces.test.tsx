import { render, screen } from '@testing-library/react';
import { 
  TracesComponent,
  TracesProject,
  TracesOrganization,
} from './index';

describe('TracesComponent', () => {
  it('renders without crashing', () => {
    render(<TracesComponent />);
    expect(screen.getByText('Traces - Component Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Title';
    render(<TracesComponent title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Description';
    render(<TracesComponent description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays component level indicator', () => {
    render(<TracesComponent />);
    expect(screen.getByText('Component Level View')).toBeInTheDocument();
  });
});

describe('TracesProject', () => {
  it('renders without crashing', () => {
    render(<TracesProject />);
    expect(screen.getByText('Traces - Project Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Project Title';
    render(<TracesProject title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Project Description';
    render(<TracesProject description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays project level indicator', () => {
    render(<TracesProject />);
    expect(screen.getByText('Project Level View')).toBeInTheDocument();
  });
});

describe('TracesOrganization', () => {
  it('renders without crashing', () => {
    render(<TracesOrganization />);
    expect(screen.getByText('Traces - Organization Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Organization Title';
    render(<TracesOrganization title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Organization Description';
    render(<TracesOrganization description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays organization level indicator', () => {
    render(<TracesOrganization />);
    expect(screen.getByText('Organization Level View')).toBeInTheDocument();
  });
});
