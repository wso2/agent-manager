/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { render, screen } from '@testing-library/react';
import { 
  OverviewComponent,
  ProjectOverview,
  OrganizationOverview,
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

describe('ProjectOverview', () => {
  it('renders without crashing', () => {
    render(<ProjectOverview />);
    expect(screen.getByText('Overview - Project Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Project Title';
    render(<ProjectOverview title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Project Description';
    render(<ProjectOverview description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays project level indicator', () => {
    render(<ProjectOverview />);
    expect(screen.getByText('Project Level View')).toBeInTheDocument();
  });
});

describe('OrganizationOverview', () => {
  it('renders without crashing', () => {
    render(<OrganizationOverview />);
    expect(screen.getByText('Overview - Organization Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Organization Title';
    render(<OrganizationOverview title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Organization Description';
    render(<OrganizationOverview description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays organization level indicator', () => {
    render(<OrganizationOverview />);
    expect(screen.getByText('Organization Level View')).toBeInTheDocument();
  });
});
