import json
# import logging
import os

from trakt import Trakt
from pyarr import SonarrAPI

from threading import Condition

# logging.basicConfig(level=logging.DEBUG)

class TraktAuthentication(object):
    def __init__(self):
        self.is_authenticating = Condition()

        self.authorization = None

        # Bind trakt events
        Trakt.on('oauth.token_refreshed', self.on_token_refreshed)

    def authenticate(self):
        if not self.is_authenticating.acquire(blocking=False):
            print('Authentication has already been started')
            return False

        # Request new device code
        code = Trakt['oauth/device'].code()

        print('Enter the code "%s" at %s to authenticate your account' % (
            code.get('user_code'),
            code.get('verification_url')
        ))

        # Construct device authentication poller
        poller = Trakt['oauth/device'].poll(**code) \
            .on('aborted', self.on_aborted) \
            .on('authenticated', self.on_authenticated) \
            .on('expired', self.on_expired) \
            .on('poll', self.on_poll)

        # Start polling for authentication token
        poller.start(daemon=False)

        # Wait for authentication to complete
        return self.is_authenticating.wait()

    def run(self):
        # test if authentication exists in file
        if os.path.exists('/data/trakt.json'):
            # use it
            with open('/data/trakt.json') as f:
                # load it
                self.authorization = json.load(f)
        else:
            self.authenticate()

        return self.authorization

    def on_aborted(self):
        """Device authentication aborted.

        Triggered when device authentication was aborted (either with `DeviceOAuthPoller.stop()`
        or via the "poll" event)
        """

        print('Authentication aborted')

        # Authentication aborted
        self.is_authenticating.acquire()
        self.is_authenticating.notify_all()
        self.is_authenticating.release()

    def on_authenticated(self, authorization):
        """Device authenticated.

        :param authorization: Authentication token details
        :type authorization: dict
        """

        # Acquire condition
        self.is_authenticating.acquire()

        # Store authorization for future calls
        self.authorization = authorization

        print('Authentication successful - authorization: %r' % self.authorization)

        # Authentication complete
        self.is_authenticating.notify_all()
        self.is_authenticating.release()

        # save authentication in a file
        with open('/data/trakt.json', 'w') as f:
            json.dump(self.authorization, f)

    def on_expired(self):
        """Device authentication expired."""

        print('Authentication expired')

        # Authentication expired
        self.is_authenticating.acquire()
        self.is_authenticating.notify_all()
        self.is_authenticating.release()

    def on_poll(self, callback):
        """Device authentication poll.

        :param callback: Call with `True` to continue polling, or `False` to abort polling
        :type callback: func
        """

        # Continue polling
        callback(True)

    def on_token_refreshed(self, authorization):
        # OAuth token refreshed, store authorization for future calls
        self.authorization = authorization

        print('Token refreshed - authorization: %r' % self.authorization)


if __name__ == '__main__':
    # Configure
    Trakt.base_url = 'https://api.trakt.tv'
    trakt_id = os.getenv('TRAKT_ID',
                         'default_client_id_here')
    trakt_secret = os.getenv('TRAKT_SECRET',
                             'default_client_secret_here')

    sonarr_url = os.getenv('SONARR_URL',
                           'default_sonarr_url_here')
    sonarr_apikey = os.getenv('SONARR_APIKEY',
                              'default_sonarr_apikey_here')
    Trakt.configuration.defaults.client(
        id=trakt_id,
        secret=trakt_secret
    )

    trakt_auth = TraktAuthentication()

    sonarr_api = SonarrAPI(host_url=sonarr_url, api_key=sonarr_apikey)

    with (Trakt.configuration.oauth.from_response(trakt_auth.run(), refresh=True)):
        for item in Trakt['sync/history'].episodes():
            serie = sonarr_api.get_series(id_=item.show.pk[1], tvdb=True)
            if serie:
                episodes = sonarr_api.get_episode(serie[0]["id"], True)
                for episode in episodes:
                    if episode['hasFile']:
                        if int(episode['tvdbId']) == int(item.to_dict().get('ids')['tvdb']):
                            print(item)
                            delete = sonarr_api.del_episode_file(episode['episodeFileId'])
