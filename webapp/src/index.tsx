import manifest from 'manifest';
import React, {useState} from 'react';

import type {AdminConsoleCustomSettingProps, PluginRegistry} from 'types/mattermost-webapp';

type ReloadStatus = 'idle' | 'loading' | 'success' | 'error';

function getCSRFToken(): string {
    const match = document.cookie.match(/(?:^|; )MMCSRF=([^;]+)/);
    return match ? decodeURIComponent(match[1]) : '';
}

function ReloadVaultButton({disabled}: AdminConsoleCustomSettingProps) {
    const [status, setStatus] = useState<ReloadStatus>('idle');
    const [message, setMessage] = useState('');

    async function handleClick() {
        setStatus('loading');
        setMessage('');
        try {
            const response = await fetch(
                `/plugins/${manifest.id}/api/v1/refresh`,
                {
                    method: 'POST',
                    credentials: 'include',
                    headers: {'X-CSRF-Token': getCSRFToken()},
                },
            );
            const data = await response.json() as {message?: string};
            if (response.ok) {
                setStatus('success');
                setMessage(data.message ?? 'Vault reloaded.');
            } else {
                setStatus('error');
                setMessage(data.message ?? `Error ${response.status}`);
            }
        } catch {
            setStatus('error');
            setMessage('Network error.');
        }
    }

    return (
        <div style={{paddingBottom: 8}}>
            <button
                type='button'
                className='btn btn-default'
                disabled={disabled || status === 'loading'}
                onClick={handleClick}
            >
                {status === 'loading' ? 'Reloading…' : 'Reload Vault'}
            </button>
            {status === 'success' && (
                <span style={{marginLeft: 12, color: '#3d9970'}}>{message}</span>
            )}
            {status === 'error' && (
                <span style={{marginLeft: 12, color: '#e74c3c'}}>{message}</span>
            )}
        </div>
    );
}

export default class Plugin {
    public initialize(registry: PluginRegistry) {
        registry.registerAdminConsoleCustomSetting('ReloadVault', ReloadVaultButton, {showTitle: true});
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());
