// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {renderHookWithContext, runPostRenderAct} from 'tests/react_testing_utils';

import {useSearchSuggestions} from './hooks';

const mockAutocompleteChannelsForSearchInTeam = jest.fn((term: string, teamId: string, success?: (channels: []) => void) => {
    return async () => {
        success?.([]);
        return {data: true};
    };
});

const mockAutocompleteUsersInTeam = jest.fn((username: string, teamId: string) => {
    return async () => {
        return {users: []};
    };
});

jest.mock('actions/channel_actions', () => ({
    autocompleteChannelsForSearchInTeam: (...args: Parameters<typeof mockAutocompleteChannelsForSearchInTeam>) => mockAutocompleteChannelsForSearchInTeam(...args),
}));

jest.mock('actions/user_actions', () => ({
    autocompleteUsersInTeam: (...args: Parameters<typeof mockAutocompleteUsersInTeam>) => mockAutocompleteUsersInTeam(...args),
}));

describe('components/new_search/hooks', () => {
    const baseState = {
        entities: {
            teams: {
                currentTeamId: 'current-team-id',
            },
        },
    };

    beforeEach(() => {
        mockAutocompleteChannelsForSearchInTeam.mockClear();
        mockAutocompleteUsersInTeam.mockClear();
    });

    test('should use current team channel autocomplete when search scope is All teams', async () => {
        renderHookWithContext(
            () => useSearchSuggestions('messages', 'in:tow', '', 6, () => 6),
            baseState,
        );

        await runPostRenderAct(2);

        expect(mockAutocompleteChannelsForSearchInTeam).toHaveBeenCalledWith(
            'tow',
            'current-team-id',
            expect.any(Function),
            undefined,
        );
    });

    test('should use current team user autocomplete when search scope is All teams', async () => {
        renderHookWithContext(
            () => useSearchSuggestions('messages', 'from:tes', '', 8, () => 8),
            baseState,
        );

        await runPostRenderAct(2);

        expect(mockAutocompleteUsersInTeam).toHaveBeenCalledWith('tes', 'current-team-id');
    });
});