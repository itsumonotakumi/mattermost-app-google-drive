# Mattermost/Google Drive Integration

* [Feature summary](#feature-summary)
* [Set up](#set-up)
    * [Installation (Plugin)](#installation-plugin)
    * [Create a Google Cloud Project](#create-a-google-cloud-project)
    * [Configuration](#configuration)
* [Admin guide](#admin-guide)
    * [Slash commands](#slash-commands)
* [End user guide](#end-user-guide)
    * [Get started](#get-started)
    * [Use /drive commands](#use-drive-commands)
* [Development](#development-environment)
    * [Building the plugin](#building-the-plugin)
    * [Manual installation](#manual-installation)

# Feature summary

This plugin allows you to integrate Google Drive to your Mattermost instance, letting you:

- Create a Google Drive file
- Share a Google Drive file
- View and reply to comments
- Publish on Google Drive any file attached to a Mattermost post
- Enable or disable notifications for all files

# Set up

## Installation (Plugin)

This integration is available as a Mattermost Plugin, which is compatible with Mattermost v9.0 and later (including v10+).

### Download and Install

1. Download the latest release from the [Releases page](https://github.com/mattermost/mattermost-app-google-drive/releases)
2. In Mattermost, go to **System Console > Plugins > Plugin Management**
3. Upload the `com.mattermost.google-drive-x.x.x.tar.gz` file
4. Enable the plugin

### Build from Source

If you want to build from source:

```bash
# Clone the repository
git clone https://github.com/mattermost/mattermost-app-google-drive.git
cd mattermost-app-google-drive

# Build the plugin
make dist

# The plugin bundle will be created at dist/com.mattermost.google-drive-x.x.x.tar.gz
```

After installation, the ``/drive`` command should be available.

## Create a Google Cloud Project

1. Create a new Project. You would need to redirect to [Google Cloud Console](https://console.cloud.google.com/home/dashboard) and select the option to **New project**. Then, select the name and the organization (optional).
2. Select APIs. After creating a project, on the left side menu on **APIs & Services**, then, select the first option **Enabled APIs & Services** and wait, the page will redirect. 
3. From the left menu select **Library** and activate: 
    - Google Drive API
    - Google Docs API
    - Google Slides API
    - Google Sheets API
    - Google Drive Activity API
4. Go back to **APIs & Services** menu.
5. Create a new OAuth consent screen. Select the option **OAuth consent screen**  on the menu bar. If you would like to limit your application to organization-only users, select **Internal**, otherwise, select **External** option, then, fill the form with the data you would use for your project.
6. Go back to **APIs & Services** menu.
7. Create a new Client. Select the option **Credentials**, and on the menu bar, select **Create credentials**, a dropdown menu will be displayed, then, select **OAuth Client ID** option. 
8. Then, a select input will ask the type of Application type that will be used, select **Web application**, then, fill the form, and on **Authorized redirect URIs** introduce this URI:
    https://<your_mattermost_instance>/plugins/com.mattermost.google-drive/oauth/complete
9. After the Client has been configured, on the main page of **Credentials**, on the submenu **OAuth 2.0 Client IDs** will be displayed the new Client and the info can be accessible whenever you need it.

## Configuration

After [installing](#installation-plugin) the plugin and [creating a project](#create-a-google-cloud-project):
1. Go to **System Console > Plugins > Google Drive**
2. Configure the following settings:
    - **Google OAuth Client ID**: The Client ID from your Google Cloud Console OAuth 2.0 credentials
    - **Google OAuth Client Secret**: The Client Secret from your Google Cloud Console OAuth 2.0 credentials
    - **Service Account Credentials (Optional)**: JSON credentials for webhook notifications
3. Click **Save** to apply the settings


# Admin guide

## Slash commands
- ``/drive help``: Shows available commands and usage information


# End user guide

## Get started

## Use ``/drive`` commands

- ``/drive help``: This command will show all current commands available for this application.
- ``/drive connect``: This command will create a new link, and clicking on that link redirects the user to select the Google account that will be used to execute all the available actions.
- ``/drive create [docs | slide | sheets]``: This command will display a new modal where the data will be asked. 
    - Title: Name of the file to be created.
    - Message: Optional. Applicable only if a file is shared in the channel.
    - File Access: Choose to share with members in the channel, with anyone who has the link, or choose to keep it private.
- ``/drive notifications [start | stop]``: This command will start or stop notification. After a user runs the ``/drive connect`` command, notifications will be posted to bot's DM channel with the user.


# Development environment

## Building the plugin

### Prerequisites

- Go 1.21 or higher
- Node.js (for version management in Makefile)
- Make

### Build from source

```bash
# Clone the repository
git clone https://github.com/mattermost/mattermost-app-google-drive.git
cd mattermost-app-google-drive

# Build the plugin for all platforms
make dist

# The plugin will be created at dist/com.mattermost.google-drive-x.x.x.tar.gz
```

### Build commands

- ``make build`` - Build the plugin for the current platform
- ``make dist`` - Build for all platforms and create the distribution tar.gz
- ``make clean`` - Remove build artifacts
- ``make test`` - Run tests
- ``make check-style`` - Run linting

## Manual installation

1. Build the plugin using ``make dist``
2. Go to **System Console > Plugins > Plugin Management**
3. Upload the generated ``.tar.gz`` file from the ``dist`` directory
4. Enable the plugin

## Deploying to a local Mattermost instance

You can deploy directly to a local Mattermost instance:

```bash
export MM_SERVICESETTINGS_SITEURL=http://localhost:8065
export MM_ADMIN_TOKEN=your-admin-token

make deploy
```
