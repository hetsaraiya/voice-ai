import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';

import { HomePage } from '@/app/pages/main/web-dashboard';

const mockGoToCreateAssistant = jest.fn();
const dashboardDesign = {
  welcome: {
    prefix: 'Welcome',
    fallbackName: 'Prashant',
  },
  hero: {
    title: 'Product Highlight: Voice AI Assistants',
    description:
      'Design, ground, deploy, and monitor real-time AI assistants across voice, web, API, and debugger channels from one workspace.',
    actions: [
      {
        label: 'Create first voice agent',
        kind: 'primary',
        intent: 'createAssistant',
      },
      {
        label: 'Talk to us',
        kind: 'secondary',
        href: 'https://cal.com/prashant-srivastav-u8duzh/30min',
        external: true,
      },
    ],
  },
  sections: [
    {
      title: 'Explore Features',
      layout: 'feature-grid',
      cards: [
        {
          title: 'AI Assistants',
          description:
            'Build branded voice agents with prompts, tools, guardrails, and versioned provider configuration.',
          action: 'Create assistant',
          href: '/deployment/assistant/create-assistant',
        },
        {
          title: 'Deployments',
          description:
            'Publish assistants to phone calls, web widgets, API endpoints, and debugger environments.',
          action: 'Manage deployments',
          href: '/deployment/assistant',
        },
        {
          title: 'Connect your provider',
          description:
            'Connect LLM, speech-to-text, text-to-speech, storage, telemetry, and credential providers.',
          action: 'Manage providers',
          href: '/integration/models',
        },
      ],
    },
    {
      title: 'Help and Resources',
      layout: 'resource-grid',
      cards: [
        {
          title: 'Documentation',
          description:
            'Explore product guides, API references, and setup docs for building with Rapida Voice AI.',
          action: 'View docs',
          href: 'https://doc.rapida.ai/',
          external: true,
        },
        {
          title: 'GitHub repository',
          description:
            'Review the open-source Voice AI repository, SDKs, examples, and implementation references.',
          action: 'Open GitHub',
          href: 'https://github.com/rapidaai/voice-ai',
          external: true,
        },
        {
          title: 'Pricing',
          description:
            'Compare plans and usage options for assistants, telephony, deployments, and platform features.',
          action: 'View pricing',
          href: 'https://www.rapida.ai/pricing',
          external: true,
        },
      ],
    },
  ],
  news: {
    title: "What's new",
    readMoreHref: '/observability/conversation',
    items: [
      {
        date: '5/21/2026',
        title: 'AgentKit and WebSocket assistant templates are ready',
        description:
          'Start from an AgentKit or WebSocket template, then configure model providers, tools, and deployment channels from one flow.',
      },
      {
        date: '5/18/2026',
        title: 'Conversation telemetry now links messages to traces',
        description:
          'Open a conversation message and inspect latency, tool calls, provider events, and execution metadata without leaving observability.',
      },
      {
        date: '5/18/2026',
        title: 'Knowledge connectors support cloud document sources',
        description:
          'Connect Google Drive, OneDrive, SharePoint, Confluence, GitHub, and Notion sources to keep assistant knowledge current.',
      },
    ],
  },
};

jest.mock('@/hooks/use-credential', () => ({
  useCurrentCredential: () => ({ user: { id: 'user-1', name: 'Prashant' } }),
}));

jest.mock('@/hooks/use-global-navigator', () => ({
  useGlobalNavigation: () => ({
    goToCreateAssistant: mockGoToCreateAssistant,
  }),
}));

jest.mock('@carbon/react', () => ({
  Button: ({
    children,
    href,
    onClick,
    className,
    renderIcon,
    ...props
  }: any) =>
    href ? (
      <a href={href} className={className} {...props}>
        {children}
      </a>
    ) : (
      <button onClick={onClick} className={className} {...props}>
        {children}
      </button>
    ),
  Link: ({ children, href, className, ...props }: any) => (
    <a href={href} className={className} {...props}>
      {children}
    </a>
  ),
  Tile: ({ children, className }: any) => (
    <section className={className}>{children}</section>
  ),
}));

describe('Dashboard HomePage', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    global.fetch = jest.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve(dashboardDesign),
      }),
    ) as jest.Mock;
  });

  it('renders the welcome dashboard structure from the hosted design', async () => {
    render(<HomePage />);

    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Welcome, Prashant!' }),
      ).toBeInTheDocument(),
    );

    expect(
      screen.getByText('Product Highlight: Voice AI Assistants'),
    ).toBeInTheDocument();
    expect(screen.getByText('Explore Features')).toBeInTheDocument();
    expect(screen.getByText('AI Assistants')).toBeInTheDocument();
    expect(screen.getByText('Deployments')).toBeInTheDocument();
    expect(screen.getByText('Connect your provider')).toBeInTheDocument();
    expect(screen.getByText('Help and Resources')).toBeInTheDocument();
    expect(screen.getByText('Documentation')).toBeInTheDocument();
    expect(screen.getByText('GitHub repository')).toBeInTheDocument();
    expect(screen.getByText('Pricing')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Talk to us' })).toHaveAttribute(
      'href',
      'https://cal.com/prashant-srivastav-u8duzh/30min',
    );
    expect(screen.getByText("What's new")).toBeInTheDocument();
    expect(screen.getAllByText(/Read more/i)).toHaveLength(3);
  });

  it('keeps the banner create action wired to assistant creation', async () => {
    render(<HomePage />);

    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Create first voice agent' }),
      ).toBeInTheDocument(),
    );

    fireEvent.click(
      screen.getByRole('button', { name: 'Create first voice agent' }),
    );

    expect(mockGoToCreateAssistant).toHaveBeenCalledTimes(1);
  });
});
