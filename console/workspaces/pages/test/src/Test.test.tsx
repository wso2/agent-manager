import { render, screen } from '@testing-library/react';
import { 
  TestComponent,
  TestProject,
  TestOrganization,
} from './index';

describe('TestComponent', () => {
  it('renders without crashing', () => {
    render(<TestComponent />);
    expect(screen.getByText('Test - Component Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Title';
    render(<TestComponent title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Description';
    render(<TestComponent description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays component level indicator', () => {
    render(<TestComponent />);
    expect(screen.getByText('Component Level View')).toBeInTheDocument();
  });
});

describe('TestProject', () => {
  it('renders without crashing', () => {
    render(<TestProject />);
    expect(screen.getByText('Test - Project Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Project Title';
    render(<TestProject title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Project Description';
    render(<TestProject description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays project level indicator', () => {
    render(<TestProject />);
    expect(screen.getByText('Project Level View')).toBeInTheDocument();
  });
});

describe('TestOrganization', () => {
  it('renders without crashing', () => {
    render(<TestOrganization />);
    expect(screen.getByText('Test - Organization Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Organization Title';
    render(<TestOrganization title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Organization Description';
    render(<TestOrganization description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays organization level indicator', () => {
    render(<TestOrganization />);
    expect(screen.getByText('Organization Level View')).toBeInTheDocument();
  });
});
