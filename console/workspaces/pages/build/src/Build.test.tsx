import { render, screen } from '@testing-library/react';
import { 
  BuildComponent,
  BuildProject,
  BuildOrganization,
} from './index';

describe('BuildComponent', () => {
  it('renders without crashing', () => {
    render(<BuildComponent />);
    expect(screen.getByText('Build - Component Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Title';
    render(<BuildComponent title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Description';
    render(<BuildComponent description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays component level indicator', () => {
    render(<BuildComponent />);
    expect(screen.getByText('Component Level View')).toBeInTheDocument();
  });
});

describe('BuildProject', () => {
  it('renders without crashing', () => {
    render(<BuildProject />);
    expect(screen.getByText('Build - Project Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Project Title';
    render(<BuildProject title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Project Description';
    render(<BuildProject description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays project level indicator', () => {
    render(<BuildProject />);
    expect(screen.getByText('Project Level View')).toBeInTheDocument();
  });
});

describe('BuildOrganization', () => {
  it('renders without crashing', () => {
    render(<BuildOrganization />);
    expect(screen.getByText('Build - Organization Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Organization Title';
    render(<BuildOrganization title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Organization Description';
    render(<BuildOrganization description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays organization level indicator', () => {
    render(<BuildOrganization />);
    expect(screen.getByText('Organization Level View')).toBeInTheDocument();
  });
});
