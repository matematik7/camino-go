# camino-go

Code for:
https://camino.ipavec.net
https://hribi.ipavec.net
https://darts.ipavec.net

For running locally you need qor admin views in `views/qor/`.

To authorize strava:
https://www.strava.com/oauth/authorize?client_id=62161&response_type=code&redirect_uri=http://localhost/exchange_token&approval_prompt=force&scope=activity:read
Use the code from the resulting url to get access and refresh token using Token Exchange instructions:
https://developers.strava.com/docs/authentication/

This respository has continuous deployment using google cloud build for all deployments.
