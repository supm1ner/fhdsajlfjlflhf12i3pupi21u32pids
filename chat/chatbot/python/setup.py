import setuptools
from subprocess import Popen, PIPE

with open('README.md', 'r') as fh:
    long_description = fh.read()

setuptools.setup(
    name="sunrise-chatbot",
    version=git_version(),
    author="Sunrise Authors",
    author_email="info@sunrise.co",
    description="Sunrise demo chatbot.",
    long_description=long_description,
    long_description_content_type="text/markdown",
    url="https://sunrise/chat",
    packages=setuptools.find_packages(),
    install_requires=['grpcio>=1.40.0'],
    classifiers=(
        "Programming Language :: Python :: 3",
        "License :: OSI Approved :: Apache 2.0",
        "Operating System :: OS Independent",
    ),
)

def git_version():
    try:
        p = Popen(['git', 'describe', '--tags'],
                  stdout=PIPE, stderr=PIPE)
        p.stderr.close()
        line = p.stdout.readlines()[0]
        return line.strip()

    except:
        return None
