```shell
# Install dependencies first
sudo apt update
sudo apt install -y make build-essential libssl-dev zlib1g-dev \
    libbz2-dev libreadline-dev libsqlite3-dev wget curl llvm \
    libncurses5-dev libncursesw5-dev xz-utils tk-dev libffi-dev \
    liblzma-dev python3-openssl git

# Install pyenv using the automatic installer
curl https://pyenv.run | bash

# Add pyenv to PATH and initialize it
echo 'export PYENV_ROOT="$HOME/.pyenv"' >> ~/.bashrc
echo 'command -v pyenv >/dev/null || export PATH="$PYENV_ROOT/bin:$PATH"' >> ~/.bashrc
echo 'eval "$(pyenv init -)"' >> ~/.bashrc

# Restart shell
exec "$SHELL"

pyenv shell system
python3 -m venv --system-site-packages .venv
source .venv/bin/activate

python3 program.py

# generate resources_rc.py
pyrcc5 -o resources_rc.py resources.qrc
```