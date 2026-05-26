import {
  Header,
  HeaderMenuItem,
  HeaderName,
  HeaderNavigation,
} from '@carbon/react';

export function GeneralFooter() {
  return (
    <Header
      aria-label="Rapida Platform"
      className="[inset-block-start:auto]! [inset-block-end:0]! border-gray-200! dark:border-gray-900! border-t!"
    >
      <HeaderName href="#" prefix="Rapida">
        [Platform]
      </HeaderName>
      <HeaderNavigation aria-label="Rapida [Platform]" className="">
        <HeaderMenuItem href="https://app.rapida.ai/static/terms-conditions">
          <span className="opacity-80">Terms and Conditions</span>
        </HeaderMenuItem>
        <HeaderMenuItem href="https://app.rapida.ai/static/privacy-policy">
          <span className="opacity-80">Privacy Policy</span>
        </HeaderMenuItem>
        <HeaderMenuItem href="https://doc.rapida.ai">
          <span className="opacity-80">Documentation</span>
        </HeaderMenuItem>
        <HeaderMenuItem href="https://github.com/rapidaai/voice-ai">
          <span className="opacity-80">Github</span>
        </HeaderMenuItem>
      </HeaderNavigation>
    </Header>
  );
}
