from contextlib import contextmanager
from mock import patch
import os
import subprocess
import tarfile
from unittest import TestCase

from crossbuild import (
    build_win_client,
    go_build,
    go_tarball,
    ISCC_CMD,
    ISS_DIR,
    make_installer,
    main,
    run_command,
    version_from_tarball,
    working_directory,
)
from utils import temp_dir


@contextmanager
def fake_go_tarball(path):
    try:
        yield path.replace('.tar.gz', '')
    finally:
        pass


class CrossBuildTestCase(TestCase):

    def test_main_setup(self):
        with patch('crossbuild.setup_cross_building') as mock:
            main(['-d', '-v', 'setup', '--build-dir', './foo'])
        args, kwargs = mock.call_args
        self.assertEqual(('./foo', ), args)
        self.assertEqual({'dry_run': True, 'verbose': True}, kwargs)

    def test_main_osx_clientt(self):
        with patch('crossbuild.build_osx_client') as mock:
            main(['osx-client', '--build-dir', './foo', 'bar.1.2.3.tar.gz'])
        args, kwargs = mock.call_args
        self.assertEqual(('bar.1.2.3.tar.gz', './foo'), args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)

    def test_main_win_client(self):
        with patch('crossbuild.build_win_client') as mock:
            main(['win-client', '--build-dir', './foo', 'bar.1.2.3.tar.gz'])
        args, kwargs = mock.call_args
        self.assertEqual(('bar.1.2.3.tar.gz', './foo'), args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)

    def test_main_win_agent(self):
        with patch('crossbuild.build_win_agent') as mock:
            main(['win-agent', '--build-dir', './foo', 'bar.1.2.3.tar.gz'])
        args, kwargs = mock.call_args
        self.assertEqual(('bar.1.2.3.tar.gz', './foo'), args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)

    def test_version_from_tarball(self):
        self.assertEqual('1.2.3', version_from_tarball('foo_1.2.3.tar.gz'))
        self.assertEqual('1.2.3', version_from_tarball('bzr/foo_1.2.3.tar.gz'))

    def test_go_build(self):
        with patch('crossbuild.run_command') as mock:
            go_build(
                'github/juju/juju/...', './foo', './bar.1.2', '386', 'windows',
                verbose=True, dry_run=True)
        args, kwargs = mock.call_args
        self.assertEqual((['go', 'build', 'github/juju/juju/...'],), args)
        self.assertTrue(kwargs['dry_run'])
        self.assertTrue(kwargs['verbose'])
        env = kwargs['env']
        self.assertEqual('./foo', env['GOROOT'])
        self.assertEqual('./bar.1.2', env['GOPATH'])
        self.assertEqual('386', env['GOARCH'])
        self.assertEqual('windows', env['GOOS'])

    def test_run_command(self):
        with patch('subprocess.check_output') as mock:
            run_command(
                ['ls'], env={'CB_MARK': 'foo'}, dry_run=False, verbose=True)
        args, kwargs = mock.call_args
        self.assertEqual((['ls'], ), args)
        self.assertEqual(
            {'env': {'CB_MARK': 'foo'}, 'stderr': subprocess.STDOUT},
            kwargs)
        with patch('subprocess.check_output') as mock:
            run_command(['ls'], dry_run=True)
        self.assertEqual(0, mock.call_count)

    def test_gotarball_raises_error(self):
        with self.assertRaises(ValueError):
            go_tarball('foo.tar.gz').__enter__()

    def test_go_tarball_gopath(self):
        with temp_dir() as base_dir:
            src_path = os.path.join(base_dir, 'juju-core_1.2.3')
            os.makedirs(src_path)
            tarball_path = '%s.tar.gz' % src_path
            with tarfile.open(tarball_path, 'w:gz') as tar:
                tar.add(src_path, arcname='juju-core_1.2.3')
            with go_tarball(tarball_path) as gopath:
                self.assertTrue(os.path.isdir(gopath))
                self.assertTrue(gopath.endswith('juju-core_1.2.3'), gopath)

    def test_working_directory(self):
        this_dir = os.getcwd()
        with temp_dir() as base_dir:
            new_dir = os.path.join(base_dir, 'juju-core_1.2.3')
            os.makedirs(new_dir)
            with working_directory(new_dir):
                self.assertEqual(new_dir, os.getcwd())
        self.assertEqual(this_dir, os.getcwd())

    def test_build_win_client(self):
        with patch('crossbuild.go_tarball',
                   side_effect=fake_go_tarball) as gt_mock:
            with patch('crossbuild.go_build') as gb_mock:
                with patch('crossbuild.make_installer') as mi_mock:
                    build_win_client('baz/bar_1.2.3.tar.gz', '/foo')
        args, kwargs = gt_mock.call_args
        self.assertEqual(('baz/bar_1.2.3.tar.gz', ), args)
        args, kwargs = gb_mock.call_args
        self.assertEqual(
            ('github.com/juju/juju/cmd/juju',
             '/foo/golang-1.2.1', 'baz/bar_1.2.3', '386', 'windows'),
            args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)
        self.assertEqual(
            ('baz/bar_1.2.3/src/github.com/juju/juju/cmd/juju/juju.exe',
             '1.2.3',
             'baz/bar_1.2.3',
             os.getcwd()),
            mi_mock.call_args[0])

    def test_make_installer_default(self):
        with temp_dir() as base_dir:
            iss_dir = os.path.join(base_dir, ISS_DIR)
            iss_output_dir = os.path.join(iss_dir, 'output')
            os.makedirs(iss_output_dir)
            with patch('shutil.move') as mv_mock:
                with patch('crossbuild.run_command') as rc_mock:
                    make_installer(
                        'foo/juju.exe', '1.2.3', base_dir, os.getcwd())
        self.assertEqual(2, mv_mock.call_count)
        # The juju.exe was moved to the iss dir.
        self.assertEqual(
            ('foo/juju.exe', iss_dir), mv_mock.mock_calls[0][1])
        args, kwargs = rc_mock.call_args
        self.assertEqual((['xvfb-run', 'wine', ISCC_CMD, 'setup.iss'], ), args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)
        # The installer was moved to the working dir.
        self.assertEqual(
            ('%s/juju-setup-1.2.3.exe' % iss_output_dir, os.getcwd()),
            mv_mock.mock_calls[1][1])

    def test_make_installer_with_dry_run(self):
        with temp_dir() as base_dir:
            iss_dir = os.path.join(base_dir, ISS_DIR)
            iss_output_dir = os.path.join(iss_dir, 'output')
            os.makedirs(iss_output_dir)
            with patch('shutil.move') as mv_mock:
                with patch('crossbuild.run_command') as rc_mock:
                    make_installer(
                        'foo/juju.exe', '1.2.3', base_dir, os.getcwd(),
                        dry_run=True, verbose=True)
        # The juju.exe was moved to the install dir, but the installer
        # was not move to the working dir.
        self.assertEqual(1, mv_mock.call_count)
        self.assertEqual(
            ('foo/juju.exe', iss_dir), mv_mock.mock_calls[0][1])
        # The installer is created in a tmp dir, so dry_run is alway False.
        self.assertEqual(
            {'dry_run': False, 'verbose': True},
            rc_mock.call_args[1])
