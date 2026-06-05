import { lazyLoad } from '@/utils/loadable';
import { LineLoader } from '@/app/components/loader/line-loader';

export const LLMActivityListingPage = lazyLoad(
  () => import('./llm-activities'),
  module => module.ListingPage,
  {
    fallback: <LineLoader />,
  },
);

export const RequestActivityListingPage = lazyLoad(
  () => import('./webhook-activities'),
  module => module.ListingPage,
  {
    fallback: <LineLoader />,
  },
);

export const ConversationActivityListingPage = lazyLoad(
  () => import('./conversation-activity-v2'),
  module => module.ListingPage,
  {
    fallback: <LineLoader />,
  },
);

export const KnowledgeActivityListingPage = lazyLoad(
  () => import('./knowledge-activities'),
  module => module.ListingPage,
  {
    fallback: <LineLoader />,
  },
);

export const ToolActivityListingPage = lazyLoad(
  () => import('./tool-activities'),
  module => module.ListingPage,
  {
    fallback: <LineLoader />,
  },
);
