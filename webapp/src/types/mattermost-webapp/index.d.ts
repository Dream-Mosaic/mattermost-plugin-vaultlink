import type {AdminConfig, ClientLicense} from '@mattermost/types/config';

export type ReactResolvable = React.ReactNode | React.ElementType;

export interface PluginRegistry {
    registerAdminConsoleCustomSetting(
        ...args: [
            key: string,
            component: ReactResolvable,
            options?: {showTitle?: boolean},
        ] | [{
            key: string;
            component: ReactResolvable;
            options?: {showTitle?: boolean};
        }]
    ): void;
}

export interface AdminConsoleCustomSettingProps {
    id: string;
    label: string;
    helpText: React.ReactNode;
    value: unknown;
    disabled: boolean;
    config: AdminConfig;
    license: ClientLicense;
    setByEnv: boolean;
    onChange: (id: string, value: unknown) => void;
    registerSaveAction: (handler: () => Promise<void>) => void;
    setSaveNeeded: () => void;
    unRegisterSaveAction: (handler: () => Promise<void>) => void;
}
